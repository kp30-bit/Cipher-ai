package usecase

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"concall-analyser/internal/domain"
	"concall-analyser/internal/infrastructure/file"
	"concall-analyser/internal/service/gemini"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (cf *concallFetcher) FetchConcallDataHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	// Parse date parameters
	fromDateStr := c.Query("from")
	toDateStr := c.Query("to")

	var fromDate, toDate time.Time
	var err error

	if fromDateStr == "" {
		fromDate = time.Now()
	} else {
		fromDate, err = parseHumanReadableDate(fromDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid 'from' date: %v", err)})
			return
		}
	}

	if toDateStr == "" {
		toDate = time.Now()
	} else {
		toDate, err = parseHumanReadableDate(toDateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid 'to' date: %v", err)})
			return
		}
	}

	if fromDate.After(toDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("'from' date (%s) cannot be after 'to' date (%s)",
				fromDate.Format("2006-01-02"), toDate.Format("2006-01-02")),
		})
		return
	}

	// Fetch announcements
	announcements, err := cf.bseClient.FetchAnnouncements(ctx, fromDate, toDate)
	if err != nil {
		log.Printf("Failed to fetch announcements: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch announcements: %v", err)})
		return
	}

	log.Printf("📊 Found %d announcements from API", len(announcements))

	if len(announcements) == 0 {
		log.Printf("⚠️ No announcements found for the given date range")
		c.JSON(http.StatusOK, gin.H{
			"message":   "No announcements found for the given date range",
			"count":     0,
			"summaries": []domain.ConcallSummary{},
		})
		return
	}

	// Filter out announcements that already exist
	filteredAnnouncements, err := cf.filterNewAnnouncements(ctx, announcements)
	if err != nil {
		log.Printf("❌ Failed to filter announcements: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to filter announcements: %v", err)})
		return
	}

	log.Printf("🆕 %d new announcements to process (out of %d total)", len(filteredAnnouncements), len(announcements))

	if len(filteredAnnouncements) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "All announcements already processed",
			"count":   0,
		})
		return
	}

	// Count announcements with PDFs
	pdfCount := 0
	for _, a := range announcements {
		if a.AttachmentName != "" {
			pdfCount++
		}
	}
	log.Printf("📄 Found %d announcements with PDFs out of %d total", pdfCount, len(announcements))

	// Create destination directory
	if err := file.CreateDirectory(cf.cfg.DestDir); err != nil {
		log.Printf("Failed to create directory: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create directory: %v", err)})
		return
	}

	// Initialize Gemini client
	geminiClient, err := gemini.NewGeminiClient(ctx, cf.cfg.APIKey)
	if err != nil {
		log.Printf("Failed to initialize Gemini client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize Gemini client: %v", err)})
		return
	}
	defer geminiClient.Close()

	// Process announcements
	log.Printf("🚀 Starting to process %d announcements...", len(filteredAnnouncements))
	summaries := cf.processAnnouncementsSequentially(ctx, geminiClient, filteredAnnouncements)
	log.Printf("✅ Finished processing. Got %d summaries", len(summaries))

	// Store summaries in MongoDB
	if len(summaries) > 0 {
		if err := cf.repo.InsertMany(ctx, summaries); err != nil {
			log.Printf("Failed to save summaries to MongoDB: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   fmt.Sprintf("Failed to save summaries to MongoDB: %v", err),
				"summary": "Processed but failed to save",
				"count":   len(summaries),
			})
			return
		}
		log.Printf("✅ Successfully inserted %d summaries to MongoDB", len(summaries))
	} else {
		log.Printf("⚠️ No summaries to save (all announcements may have been skipped)")
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Announcements processed and saved successfully",
		"count":     len(summaries),
		"summaries": summaries,
	})
}

func (cf *concallFetcher) filterNewAnnouncements(ctx context.Context, announcements []domain.Announcement) ([]domain.Announcement, error) {
	if len(announcements) == 0 {
		return []domain.Announcement{}, nil
	}

	names := make([]string, 0, len(announcements))
	for _, a := range announcements {
		names = append(names, a.ShortLongName)
	}

	existingNames, err := cf.repo.FindExistingNames(ctx, names)
	if err != nil {
		return nil, err
	}

	filtered := make([]domain.Announcement, 0, len(announcements))
	for _, a := range announcements {
		if !existingNames[a.ShortLongName] {
			filtered = append(filtered, a)
		} else {
			log.Printf("🗑️ Skipping existing announcement: %s", a.ShortLongName)
		}
	}

	return filtered, nil
}

func (cf *concallFetcher) processAnnouncementsSequentially(
	ctx context.Context,
	geminiClient gemini.GeminiClient,
	announcements []domain.Announcement,
) []domain.ConcallSummary {
	results := make([]domain.ConcallSummary, 0)
	skippedCount := 0
	errorCount := 0

	log.Printf("⚙️ Starting sequential processing of %d announcements...", len(announcements))

	for i, a := range announcements {
		log.Printf("🔹 [%d/%d] Processing: %s", i+1, len(announcements), a.ShortLongName)

		summary, err := cf.processAnnouncement(ctx, geminiClient, a)

		if err != nil {
			log.Printf("❌ Error processing announcement %s (PDFFlag: %d, Attachment: %s): %v",
				a.ShortLongName, a.PDFFlag, a.AttachmentName, err)
			errorCount++
			continue
		}

		if summary != nil {
			// Remove "-$" suffix from name if present
			if strings.HasSuffix(summary.Name, "-$") {
				summary.Name = strings.TrimSuffix(summary.Name, "-$")
			}

			results = append(results, *summary)
			log.Printf("✅ Processed successfully: %s", a.ShortLongName)
		} else {
			skippedCount++
			log.Printf("⏭️ Skipped announcement: %s (PDFFlag: %d, Attachment: %s)",
				a.ShortLongName, a.PDFFlag, a.AttachmentName)
		}

		time.Sleep(1 * time.Second)
	}

	log.Printf("📈 Processing complete - Success: %d, Skipped: %d, Errors: %d",
		len(results), skippedCount, errorCount)

	return results
}

func (cf *concallFetcher) processAnnouncement(ctx context.Context, geminiClient gemini.GeminiClient, a domain.Announcement) (*domain.ConcallSummary, error) {
	if a.AttachmentName == "" {
		log.Printf("⏭️ Skipping announcement AttachmentName='%s'", a.ShortLongName)
		return nil, nil
	}

	datePart := strings.Split(a.NewsDate, "T")[0]
	companyPart := file.SanitizeFileName(a.ShortLongName)
	saveAs := fmt.Sprintf("%s_%s.pdf", companyPart, datePart)

	log.Printf("📥 Downloading PDF: %s (from %s)", saveAs, a.AttachmentName)
	path, err := cf.pdfDownloader.Download(ctx, a.AttachmentName, cf.cfg.DestDir, saveAs)
	if err != nil {
		return nil, fmt.Errorf("download error for %s: %w", saveAs, err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file stat error for %s: %w", path, err)
	}
	if fileInfo.Size() == 0 {
		return nil, fmt.Errorf("PDF file is empty at %s", path)
	}

	log.Printf("✅ PDF saved to %s (size: %d bytes)", path, fileInfo.Size())

	defer func() {
		if err := os.Remove(path); err != nil {
			log.Printf("⚠️ Warning: failed to remove temp file %s: %v", path, err)
		}
	}()

	log.Printf("🤖 Uploading and summarizing PDF: %s", saveAs)
	summary, err := geminiClient.SummarizePDF(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("summarization error for %s: %w", saveAs, err)
	}
	log.Printf("✅ Summary generated for %s:", saveAs)

	concallSummary := &domain.ConcallSummary{
		ID:        primitive.NewObjectID(),
		Name:      a.ShortLongName,
		Date:      datePart,
		Guidance:  summary,
		CreatedAt: time.Now(),
	}

	return concallSummary, nil
}

// parseHumanReadableDate parses a human-readable date string into time.Time
func parseHumanReadableDate(dateStr string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"02-01-2006",
		"01/02/2006",
		"02/01/2006",
		"20060102",
		"2006-1-2",
		"2-1-2006",
		"January 2, 2006",
		"2 January 2006",
		time.RFC3339,
		time.RFC3339Nano,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date '%s'. Supported formats: YYYY-MM-DD, DD-MM-YYYY, MM/DD/YYYY, DD/MM/YYYY, YYYYMMDD", dateStr)
}
