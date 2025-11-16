package usecase

import (
	"concall-analyser/config"
	"concall-analyser/internal/db"
	"concall-analyser/internal/domain"
	"concall-analyser/internal/interfaces"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/generative-ai-go/genai"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type concallFetcher struct {
	db  *db.MongoDB
	cfg *config.Config
}

type HTTPClient struct {
	*http.Client
}

func NewHTTPClient() *HTTPClient {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   3600 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   3600 * time.Second,
			KeepAlive: 3600 * time.Second,
		}).DialContext,
	}

	return &HTTPClient{
		Client: &http.Client{
			Timeout:   3600 * time.Second,
			Transport: tr,
		},
	}
}

func NewConcallFetcher(db *db.MongoDB, cfg *config.Config) interfaces.Usecase {
	return &concallFetcher{db: db, cfg: cfg}
}

func (cf *concallFetcher) FetchConcallDataHandler(c *gin.Context) {

	// Create optimized HTTP client
	httpClient := NewHTTPClient()
	// Fetch announcements
	announcements, err := fetchAnnouncements(httpClient, c)
	if err != nil {
		log.Printf("Failed to fetch announcements: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch announcements: %v", err)})
		return
	}

	log.Printf("📊 Found %d announcements from API", len(announcements))

	// Check if we have any announcements
	if len(announcements) == 0 {
		log.Printf("⚠️ No announcements found for the given date range")
		c.JSON(http.StatusOK, gin.H{
			"message":   "No announcements found for the given date range",
			"count":     0,
			"summaries": []domain.ConcallSummary{},
		})
		return
	}
	// 2️⃣ Filter out announcements that already exist in Mongo
	coll := cf.db.Database.Collection("guidances")
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	filteredAnnouncements, err := filterNewAnnouncements(ctx, coll, announcements)
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
	if err := os.MkdirAll(cf.cfg.DestDir, 0755); err != nil {
		log.Printf("Failed to create directory: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create directory: %v", err)})
		return
	}

	// Set default MaxWorkers if not configured
	maxWorkers := cf.cfg.MaxWorkers
	if maxWorkers == 0 {
		maxWorkers = 10 // Default to 10 workers
		log.Printf("⚠️ MaxWorkers not set, using default: %d", maxWorkers)
	}
	log.Printf("🔧 Using %d concurrent workers", maxWorkers)

	genaiClient, model, err := initializeGeminiClient(ctx, cf.cfg.APIKey)
	if err != nil {
		log.Printf("Failed to initialize Gemini client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to initialize Gemini client: %v", err)})
		return
	}
	defer genaiClient.Close()

	log.Printf("🚀 Starting to process %d announcements...", len(announcements))
	// Process announcements concurrently and get results
	summaries := processAnnouncementsSequentially(ctx, httpClient, genaiClient, model, announcements, cf.cfg.DestDir)
	log.Printf("✅ Finished processing. Got %d summaries", len(summaries))

	// Store summaries in MongoDB
	if len(summaries) > 0 {
		if err := cf.saveSummariesToMongo(ctx, summaries); err != nil {
			log.Printf("Failed to save summaries to MongoDB: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   fmt.Sprintf("Failed to save summaries to MongoDB: %v", err),
				"summary": "Processed but failed to save",
				"count":   len(summaries),
			})
			return
		}
	} else {
		log.Printf("⚠️ No summaries to save (all announcements may have been skipped)")
	}

	// Return success response with summaries
	c.JSON(http.StatusOK, gin.H{
		"message":   "Announcements processed and saved successfully",
		"count":     len(summaries),
		"summaries": summaries,
	})
}

func filterNewAnnouncements(ctx context.Context, coll *mongo.Collection, announcements []domain.Announcement) ([]domain.Announcement, error) {
	if len(announcements) == 0 {
		return []domain.Announcement{}, nil
	}

	// Collect all names
	names := make([]string, 0, len(announcements))
	for _, a := range announcements {
		names = append(names, a.ShortLongName)
	}

	// Fetch existing documents by name
	filter := bson.M{"name": bson.M{"$in": names}}
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("mongo find error: %w", err)
	}
	defer cursor.Close(ctx)

	existingNames := make(map[string]bool)
	for cursor.Next(ctx) {
		var doc struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&doc); err == nil {
			existingNames[doc.Name] = true
		}
	}

	// Filter out announcements that already exist
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

// saveSummariesToMongo stores the concall summaries in MongoDB
func (cf *concallFetcher) saveSummariesToMongo(ctx context.Context, summaries []domain.ConcallSummary) error {
	if cf.db == nil {
		return fmt.Errorf("MongoDB database is not initialized")
	}

	collection := cf.db.Collection("guidances")

	// Convert summaries to interface slice for bulk insert
	docs := make([]interface{}, len(summaries))
	for i, summary := range summaries {
		docs[i] = summary
	}

	// Use InsertMany for bulk insert
	result, err := collection.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert summaries: %w", err)
	}

	log.Printf("✅ Successfully inserted %d summaries to MongoDB", len(result.InsertedIDs))
	return nil
}

func processAnnouncementsSequentially(
	ctx context.Context,
	httpClient *HTTPClient,
	genaiClient *genai.Client,
	model *genai.GenerativeModel,
	announcements []domain.Announcement,
	destDir string,
) []domain.ConcallSummary {

	results := make([]domain.ConcallSummary, 0)
	skippedCount := 0
	errorCount := 0

	log.Printf("⚙️ Starting sequential processing of %d announcements...", len(announcements))

	for i, a := range announcements {
		log.Printf("🔹 [%d/%d] Processing: %s", i+1, len(announcements), a.ShortLongName)

		summary, err := processAnnouncement(ctx, httpClient, genaiClient, model, a, destDir)

		if err != nil {
			log.Printf("❌ Error processing announcement %s (PDFFlag: %d, Attachment: %s): %v",
				a.ShortLongName, a.PDFFlag, a.AttachmentName, err)
			errorCount++
			continue
		}

		// Only add to results if summary is valid (not nil)
		if summary != nil {
			results = append(results, *summary)
			log.Printf("✅ Processed successfully: %s", a.ShortLongName)
		} else {
			skippedCount++
			log.Printf("⏭️ Skipped announcement: %s (PDFFlag: %d, Attachment: %s)",
				a.ShortLongName, a.PDFFlag, a.AttachmentName)
		}

		// Optional delay between each call (e.g., 10 seconds)
		time.Sleep(1 * time.Second)
	}

	log.Printf("📈 Processing complete - Success: %d, Skipped: %d, Errors: %d",
		len(results), skippedCount, errorCount)

	return results
}

func processAnnouncement(ctx context.Context, client *HTTPClient, genaiClient *genai.Client, model *genai.GenerativeModel, a domain.Announcement, destDir string) (*domain.ConcallSummary, error) {
	// Only download if PDF exists
	if a.AttachmentName == "" {
		log.Printf("⏭️ Skipping announcement  AttachmentName='%s'", a.ShortLongName)
		return nil, nil // Return nil to skip, not an error
	}

	datePart := strings.Split(a.NewsDate, "T")[0]
	companyPart := sanitizeFileName(a.ShortLongName)
	saveAs := fmt.Sprintf("%s_%s.pdf", companyPart, datePart)

	log.Printf("📥 Downloading PDF: %s (from %s)", saveAs, a.AttachmentName)
	path, err := downloadPDF(client, a.AttachmentName, destDir, saveAs)
	if err != nil {
		return nil, fmt.Errorf("download error for %s: %w", saveAs, err)
	}

	// Check if file exists and is not empty
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file stat error for %s: %w", path, err)
	}
	if fileInfo.Size() == 0 {
		return nil, fmt.Errorf("PDF file is empty at %s", path)
	}

	log.Printf("✅ PDF saved to %s (size: %d bytes)", path, fileInfo.Size())

	// Ensure local temp file is cleaned up after processing
	defer func() {
		if err := os.Remove(path); err != nil {
			log.Printf("⚠️ Warning: failed to remove temp file %s: %v", path, err)
		}
	}()

	// Upload and summarize with Gemini
	log.Printf("🤖 Uploading and summarizing PDF: %s", saveAs)
	summary, err := summarizePDFWithGemini(ctx, genaiClient, model, path, saveAs)
	if err != nil {
		return nil, fmt.Errorf("summarization error for %s: %w", saveAs, err)
	}
	log.Printf("✅ Summary generated for %s:", saveAs)

	// Create and return ConcallSummary struct
	concallSummary := &domain.ConcallSummary{
		ID:        primitive.NewObjectID(),
		Name:      a.ShortLongName,
		Date:      datePart,
		Guidance:  summary,
		CreatedAt: time.Now(),
	}

	return concallSummary, nil
}

func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

func downloadPDF(client *HTTPClient, attachmentName, destDir, saveAs string) (string, error) {
	if attachmentName == "" {
		return "", fmt.Errorf("attachment name is empty")
	}

	baseURL := "https://www.bseindia.com/xml-data/corpfiling/AttachLive/"
	fullURL := baseURL + attachmentName

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bseindia.com/")
	req.Header.Set("Accept", "application/pdf")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download PDF: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download %s, status %d", attachmentName, resp.StatusCode)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(destDir, saveAs)
	out, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

func makeGeminiCallWithRetry(ctx context.Context, model *genai.GenerativeModel, file *genai.File, prompt string) (*genai.GenerateContentResponse, error) {
	const maxRetries = 5
	baseDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		resp, err := model.GenerateContent(ctx,
			genai.FileData{MIMEType: file.MIMEType, URI: file.URI},
			genai.Text(prompt),
		)

		if err == nil {
			return resp, nil // Success
		}

		// Check if the error is one we should retry (e.g., rate limit)
		if !isRetriableError(err) {
			return nil, fmt.Errorf("Gemini generation failed with non-retriable error: %w", err)
		}

		// Calculate exponential backoff delay with jitter
		// Formula: baseDelay * 2^i + jitter
		// time.Duration(1<<i) is equivalent to 2^i
		delay := baseDelay * time.Duration(1<<i)
		// Add some random jitter (up to 20% of the delay) to prevent thundering herd problem
		jitter := time.Duration(rand.Int63n(int64(delay) / 5))
		sleepTime := delay + jitter

		log.Printf("⚠️ Rate limit or transient error detected. Retrying in %v (Attempt %d/%d). Error: %v", sleepTime, i+1, maxRetries, err)

		// Wait for the calculated backoff time
		select {
		case <-ctx.Done():
			return nil, ctx.Err() // Context canceled before retry
		case <-time.After(sleepTime):
			// Continue to the next loop iteration to retry
		}
	}

	return nil, fmt.Errorf("Gemini generation failed after %d retries due to rate limits/transient errors", maxRetries)
}
func isRetriableError(err error) bool {
	if apiErr, ok := err.(*googleapi.Error); ok {
		// HTTP 429 Too Many Requests (Rate Limit)
		// HTTP 500 Internal Server Error (Transient backend issue)
		// HTTP 503 Service Unavailable (Transient backend issue)
		return apiErr.Code == 429 || apiErr.Code == 500 || apiErr.Code == 503
	}
	// Default to not retriable if not a Google API error
	return false
}
func summarizePDFWithGemini(ctx context.Context, client *genai.Client, model *genai.GenerativeModel, pdfPath, saveAs string) (string, error) {
	// Upload file using the optimized method
	file, err := client.UploadFileFromPath(ctx, pdfPath, &genai.UploadFileOptions{
		MIMEType: "application/pdf",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload PDF: %w", err)
	}

	fmt.Printf("✅ Uploaded file: %s (MIME: %s)\n", file.Name, file.MIMEType)

	prompt := `Go through the concall and identify if management has given any guidance for fy26 on the future growth of the company in terms of revenue, earnings, eps etc. If yes, then just return the fy26' guidance after quantifying it and return nothing else. If no guidance is provided, then return "NA". Your responsse should be just 1 line providing the guidance for fy26' in numbers otherwise NA.`
	resp, err := makeGeminiCallWithRetry(ctx, model, file, prompt)
	// resp, err := model.GenerateContent(ctx,
	// 	genai.FileData{
	// 		MIMEType: file.MIMEType,
	// 		URI:      file.URI,
	// 	},
	// 	genai.Text(prompt),
	// )
	if err != nil {
		return "", fmt.Errorf("Gemini generation failed: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return "(no response)", nil
	}

	var output strings.Builder
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			output.WriteString(fmt.Sprintln(part))
		}
	}

	// Clean up uploaded file
	if err := client.DeleteFile(ctx, file.Name); err != nil {
		log.Printf("Warning: failed to delete uploaded file %s: %v", file.Name, err)
	}

	return strings.TrimSpace(output.String()), nil
}

func initializeGeminiClient(ctx context.Context, apiKey string) (*genai.Client, *genai.GenerativeModel, error) {
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := genaiClient.GenerativeModel("gemini-2.5-flash")
	return genaiClient, model, nil
}

// parseHumanReadableDate parses a human-readable date string into time.Time
// Supports multiple formats: "2025-10-18", "18-10-2025", "10/18/2025", "20251018"
func parseHumanReadableDate(dateStr string) (time.Time, error) {
	// Common date formats to try
	formats := []string{
		"2006-01-02",      // ISO format: 2025-10-18
		"02-01-2006",      // DD-MM-YYYY: 18-10-2025
		"01/02/2006",      // MM/DD/YYYY: 10/18/2025
		"02/01/2006",      // DD/MM/YYYY: 18/10/2025
		"20060102",        // YYYYMMDD: 20251018 (existing format)
		"2006-1-2",        // ISO format without leading zeros
		"2-1-2006",        // DD-MM-YYYY without leading zeros
		"January 2, 2006", // "October 18, 2025"
		"2 January 2006",  // "18 October 2025"
		time.RFC3339,      // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano,  // "2006-01-02T15:04:05.999999999Z07:00"
	}

	// Try each format
	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	// If all formats fail, return error
	return time.Time{}, fmt.Errorf("unable to parse date '%s'. Supported formats: YYYY-MM-DD, DD-MM-YYYY, MM/DD/YYYY, DD/MM/YYYY, YYYYMMDD", dateStr)
}

// Fetch all pages between fromDate and toDate, accumulating results.
func fetchAnnouncements(client *HTTPClient, c *gin.Context) ([]domain.Announcement, error) {
	fromDateStr := c.Query("from")
	toDateStr := c.Query("to")

	var fromDate, toDate time.Time
	var err error

	// Parse fromDate
	if fromDateStr == "" {
		fromDate = time.Now()
	} else {
		fromDate, err = parseHumanReadableDate(fromDateStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'from' date: %w", err)
		}
	}

	// Parse toDate
	if toDateStr == "" {
		toDate = time.Now()
	} else {
		toDate, err = parseHumanReadableDate(toDateStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'to' date: %w", err)
		}
	}

	// Validate date range (fromDate should not be after toDate)
	if fromDate.After(toDate) {
		return nil, fmt.Errorf("'from' date (%s) cannot be after 'to' date (%s)",
			fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	}

	// Format dates as YYYYMMDD for the API
	fromDateFormatted := fromDate.Format("20060102")
	toDateFormatted := toDate.Format("20060102")

	baseURL := "https://api.bseindia.com/BseIndiaAPI/api/AnnSubCategoryGetData/w" +
		"?pageno=1&strCat=Company+Update&strPrevDate=20251018&strScrip=&strSearch=P" +
		"&strToDate=20251018&strType=C&subcategory=Earnings+Call+Transcript"

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("strPrevDate", fromDateFormatted)
	q.Set("strToDate", toDateFormatted)
	q.Set("pageno", "1") // Fetch only page 1
	u.RawQuery = q.Encode()

	// Make request for page 1
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://www.bseindia.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Sec-CH-UA", `"Google Chrome";v="141", "Not?A_Brand";v="8", "Chromium";v="141"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch announcements: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("BSE API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var ar domain.AnnouncementResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Return the announcements from the first page
	return ar.Table, nil
}

// Helper function for min (if not using Go 1.21+)

func (cf *concallFetcher) ListConcallHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "12")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 12
	}

	skip := int64((page - 1) * limit)
	limit64 := int64(limit)

	coll := cf.db.Collection("guidances")

	// Filter to exclude documents where guidance is "NA"
	filter := bson.M{
		"guidance": bson.M{"$ne": "NA"},
	}

	// We only need name, date, guidance
	projection := bson.M{
		"name":     1,
		"date":     1,
		"guidance": 1,
		"_id":      0, // don't return _id if not needed
	}

	findOpts := options.Find().
		SetProjection(projection).
		SetSort(bson.D{{Key: "date", Value: -1}}). // newest first
		SetSkip(skip).
		SetLimit(limit64)

	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to query MongoDB",
			"details": err.Error(),
		})
		return
	}
	defer cursor.Close(ctx)

	var results []domain.ConcallLite
	if err := cursor.All(ctx, &results); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to decode documents",
			"details": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to count documents",
			"details": err.Error(),
		})
		return
	}

	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"meta": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": totalPages,
		},
		"data": results,
	})
}

func (cf *concallFetcher) FindConcallHandler(c *gin.Context) {
	// Read and sanitize input
	rawName := c.Query("name")
	if strings.TrimSpace(rawName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'name' is required"})
		return
	}
	// Replace + (some clients may send + for spaces), trim spaces
	name := strings.TrimSpace(strings.ReplaceAll(rawName, "+", " "))

	// Pagination params (optional)
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "12")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 12
	}
	skip := int64((page - 1) * limit)
	limit64 := int64(limit)

	// Build context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	coll := cf.db.Collection("guidances")

	// Use regexp.QuoteMeta to escape any regex metacharacters the user may send.
	// We do a substring match (no ^ or $ anchors) and add case-insensitive option.
	escaped := regexp.QuoteMeta(name)
	// The regex value is the escaped string; we set options "i" for case-insensitive
	filter := bson.M{
		"name": bson.M{
			"$regex":   escaped,
			"$options": "i",
		},
	}

	// Projection: only return the fields we care about
	projection := bson.M{
		"name":     1,
		"date":     1,
		"guidance": 1,
		"_id":      0,
	}

	findOpts := options.Find().
		SetProjection(projection).
		SetSort(bson.D{{Key: "date", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit64)

	// Count total matching docs (for pagination meta)
	totalCount, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count documents", "details": err.Error()})
		return
	}

	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query MongoDB", "details": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	var results []domain.ConcallLite
	if err := cursor.All(ctx, &results); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode documents", "details": err.Error()})
		return
	}

	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"meta": gin.H{
			"query":      name,
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": totalPages,
		},
		"data": results,
	})

}

func (cf *concallFetcher) CleanupConcallHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	coll := cf.db.Collection("guidances")

	// Step 1: Delete all records with guidance == "NA"
	naFilter := bson.M{"guidance": "NA"}
	naResult, err := coll.DeleteMany(ctx, naFilter)
	if err != nil {
		log.Printf("❌ Failed to delete NA guidance records: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to delete NA guidance records",
			"details": err.Error(),
		})
		return
	}
	naDeletedCount := naResult.DeletedCount
	log.Printf("🗑️ Deleted %d records with guidance='NA'", naDeletedCount)

	// Step 2: Find and delete duplicates based on name field
	// We'll keep the most recent record (by created_at) for each name

	// First, get all documents grouped by name
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id": "$name",
				"docs": bson.M{
					"$push": "$$ROOT",
				},
				"count": bson.M{"$sum": 1},
			},
		},
		{
			"$match": bson.M{
				"count": bson.M{"$gt": 1}, // Only groups with more than 1 document
			},
		},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("❌ Failed to find duplicates: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to find duplicates",
			"details": err.Error(),
		})
		return
	}
	defer cursor.Close(ctx)

	type DuplicateGroup struct {
		Name  string                  `bson:"_id"`
		Docs  []domain.ConcallSummary `bson:"docs"`
		Count int                     `bson:"count"`
	}

	var duplicateGroups []DuplicateGroup
	if err := cursor.All(ctx, &duplicateGroups); err != nil {
		log.Printf("❌ Failed to decode duplicate groups: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to decode duplicate groups",
			"details": err.Error(),
		})
		return
	}

	duplicateDeletedCount := int64(0)
	duplicateNamesProcessed := 0

	// For each duplicate group, keep the most recent one and delete the rest
	for _, group := range duplicateGroups {
		if len(group.Docs) <= 1 {
			continue
		}

		// Find the document with the most recent created_at
		var keepID primitive.ObjectID
		var latestTime time.Time

		for _, doc := range group.Docs {
			if doc.CreatedAt.After(latestTime) || latestTime.IsZero() {
				latestTime = doc.CreatedAt
				keepID = doc.ID
			}
		}

		// Delete all documents with this name except the one we're keeping
		deleteFilter := bson.M{
			"name": group.Name,
			"_id":  bson.M{"$ne": keepID},
		}

		deleteResult, err := coll.DeleteMany(ctx, deleteFilter)
		if err != nil {
			log.Printf("⚠️ Failed to delete duplicates for name '%s': %v", group.Name, err)
			continue
		}

		duplicateDeletedCount += deleteResult.DeletedCount
		duplicateNamesProcessed++
		log.Printf("🗑️ Deleted %d duplicate(s) for name '%s' (kept most recent)", deleteResult.DeletedCount, group.Name)
	}

	totalDeleted := naDeletedCount + duplicateDeletedCount

	log.Printf("✅ Cleanup complete - NA records deleted: %d, Duplicates deleted: %d, Total deleted: %d",
		naDeletedCount, duplicateDeletedCount, totalDeleted)

	c.JSON(http.StatusOK, gin.H{
		"message": "Cleanup completed successfully",
		"summary": gin.H{
			"naGuidanceDeleted":       naDeletedCount,
			"duplicatesDeleted":       duplicateDeletedCount,
			"duplicateNamesProcessed": duplicateNamesProcessed,
			"totalDeleted":            totalDeleted,
		},
	})
}
