package usecase

import (
	"concall-analyser/config"
	"concall-analyser/internal/db"
	"concall-analyser/internal/interfaces"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type concallFetcher struct {
	db  *db.MongoDB
	cfg *config.Config
}

type HTTPClient struct {
	*http.Client
}

type AnnouncementResponse struct {
	Table  []Announcement `json:"Table"`
	Table1 []struct {
		ROWCNT int `json:"ROWCNT"`
	} `json:"Table1"`
}
type Announcement struct {
	NewsID           string  `json:"NEWSID"`
	ScripCode        int     `json:"SCRIP_CD"`
	XMLName          string  `json:"XML_NAME"`
	NewsSubject      string  `json:"NEWSSUB"`
	Datetime         string  `json:"DT_TM"`
	NewsDate         string  `json:"NEWS_DT"`
	NewsSubmission   string  `json:"News_submission_dt"`
	DisseminationDT  string  `json:"DissemDT"`
	CriticalNews     int     `json:"CRITICALNEWS"`
	AnnouncementType string  `json:"ANNOUNCEMENT_TYPE"`
	QuarterID        *string `json:"QUARTER_ID"`
	FileStatus       string  `json:"FILESTATUS"`
	AttachmentName   string  `json:"ATTACHMENTNAME"`
	More             string  `json:"MORE"`
	Headline         string  `json:"HEADLINE"`
	CategoryName     string  `json:"CATEGORYNAME"`
	Old              int     `json:"OLD"`
	RN               int     `json:"RN"`
	PDFFlag          int     `json:"PDFFLAG"`
	NSURL            string  `json:"NSURL"`
	ShortLongName    string  `json:"SLONGNAME"`
	AgendaID         int     `json:"AGENDA_ID"`
	TotalPageCount   int     `json:"TotalPageCnt"`
	TimeDiff         string  `json:"TimeDiff"`
	FileAttachSize   int64   `json:"Fld_Attachsize"`
	SubCategoryName  string  `json:"SUBCATNAME"`
	AudioVideoFile   *string `json:"AUDIO_VIDEO_FILE"`
}

// NewHTTPClient creates an optimized HTTP client
func NewHTTPClient() *HTTPClient {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: false,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &HTTPClient{
		Client: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}
}

func NewConcallFetcher(db *db.MongoDB, cfg *config.Config) interfaces.Usecase {
	return &concallFetcher{db: db, cfg: cfg}
}

func (cf *concallFetcher) FetchConcallDataHandler(c *gin.Context) {
	// TODO: Implement FetchConcallData
	if cf.cfg.APIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key is required"})
		return
	}

	if cf.cfg.BaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Base URL is required"})
		return
	}

	if cf.cfg.DestDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Destination directory is required"})
		return
	}

	if cf.cfg.MaxWorkers == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum workers is required"})
		return
	}

	// Create optimized HTTP client
	httpClient := NewHTTPClient()
	// Fetch announcements
	announcements, err := fetchAnnouncements(httpClient, c)
	if err != nil {
		log.Fatalf("Failed to fetch announcements: %v", err)
	}

	fmt.Printf("Found %d announcements\n", len(announcements))
	if err := os.MkdirAll(cf.cfg.DestDir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{
		"message":       "Announcements fetched successfully",
		"count":         len(announcements),
		"announcements": announcements,
	})
}

// func fetchAnnouncements(client *HTTPClient, c *gin.Context) ([]Announcement, error) {
// 	fromDate := c.Query("from")
// 	toDate := c.Query("to")

// 	// Default to today (UTC/local as per server clock)
// 	if fromDate == "" {
// 		fromDate = time.Now().Format("20060102")
// 	}
// 	if toDate == "" {
// 		toDate = time.Now().Format("20060102")
// 	}

// 	// Validate and normalize dates; swap if from > to
// 	fd, err := time.Parse("20060102", fromDate)
// 	if err != nil {
// 		return nil, fmt.Errorf("invalid 'from' date (want YYYYMMDD): %w", err)
// 	}
// 	td, err := time.Parse("20060102", toDate)
// 	if err != nil {
// 		return nil, fmt.Errorf("invalid 'to' date (want YYYYMMDD): %w", err)
// 	}
// 	if fd.After(td) {
// 		fd, td = td, fd
// 		fromDate = fd.Format("20060102")
// 		toDate = td.Format("20060102")
// 	}

// 	// Base URL with stable shape (dates will be replaced)
// 	rawURL := "https://api.bseindia.com/BseIndiaAPI/api/AnnSubCategoryGetData/w?" +
// 		"pageno=1&strCat=Company+Update&strPrevDate=20251018&strScrip=&strSearch=P&strToDate=20251018&strType=C&subcategory=Earnings+Call+Transcript"

// 	parsedURL, err := url.Parse(rawURL)
// 	if err != nil {
// 		return nil, fmt.Errorf("invalid base URL: %w", err)
// 	}

// 	// Modify only date parameters
// 	q := parsedURL.Query()
// 	q.Set("strPrevDate", fromDate)
// 	q.Set("strToDate", toDate)
// 	parsedURL.RawQuery = q.Encode()

// 	finalURL := parsedURL.String() // avoid shadowing net/url

// 	// Use request context from Gin so it cancels on client disconnect/timeouts
// 	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, finalURL, nil)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create request: %w", err)
// 	}

// 	// Headers to look like a browser (optional to keep)
// 	req.Header.Set("Accept", "application/json, text/plain, */*")
// 	req.Header.Set("Referer", "https://www.bseindia.com/")
// 	req.Header.Set("User-Agent", "Mozilla/5.0")
// 	req.Header.Set("Sec-CH-UA", `"Google Chrome";v="141", "Not?A_Brand";v="8", "Chromium";v="141"`)
// 	req.Header.Set("Sec-CH-UA-Mobile", "?0")
// 	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)

// 	resp, err := client.Do(req)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to fetch announcements: %w", err)
// 	}
// 	defer resp.Body.Close()

// 	if resp.StatusCode != http.StatusOK {
// 		return nil, fmt.Errorf("BSE API returned status %d", resp.StatusCode)
// 	}

// 	body, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to read response body: %w", err)
// 	}

// 	var annResp AnnouncementResponse
// 	if err := json.Unmarshal(body, &annResp); err != nil {
// 		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
// 	}

// 	// Normalize nil to empty slice
// 	if annResp.Table == nil {
// 		return []Announcement{}, nil
// 	}
// 	return annResp.Table, nil
// }

// Fetch all pages between fromDate and toDate, accumulating results.
func fetchAnnouncements(client *HTTPClient, c *gin.Context) ([]Announcement, error) {
	fromDate := c.Query("from")
	toDate := c.Query("to")
	if fromDate == "" {
		fromDate = time.Now().Format("20060102")
	}
	if toDate == "" {
		toDate = time.Now().Format("20060102")
	}

	baseURL := "https://api.bseindia.com/BseIndiaAPI/api/AnnSubCategoryGetData/w" +
		"?pageno=1&strCat=Company+Update&strPrevDate=20251018&strScrip=&strSearch=P" +
		"&strToDate=20251018&strType=C&subcategory=Earnings+Call+Transcript"

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("strPrevDate", fromDate)
	q.Set("strToDate", toDate)
	u.RawQuery = q.Encode()

	const maxPages = 200 // safety guard; tune if needed
	all := make([]Announcement, 0, 256)

	for page := 1; page <= maxPages; page++ {
		// set current page
		q := u.Query()
		q.Set("pageno", strconv.Itoa(page))
		u.RawQuery = q.Encode()

		// GET page
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
			return nil, fmt.Errorf("fetch page %d: %w", page, err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("BSE API status %d on page %d", resp.StatusCode, page)
				return
			}
			body, e := io.ReadAll(resp.Body)
			if e != nil {
				err = fmt.Errorf("read body page %d: %w", page, e)
				return
			}

			var ar AnnouncementResponse
			if e := json.Unmarshal(body, &ar); e != nil {
				err = fmt.Errorf("unmarshal page %d: %w", page, e)
				return
			}

			rows := ar.Table
			// Stop condition: empty page
			if len(rows) == 0 {
				// no more pages
				return
			}

			all = append(all, rows...)
		}()
		if err != nil {
			return nil, err
		}

		// Optional: be polite if they rate-limit
		time.Sleep(150 * time.Millisecond)
	}

	// Optional: de-duplicate if the API can return overlaps
	// (e.g., by NewsID field if available)
	// all = dedupeAnnouncements(all)

	return all, nil
}
