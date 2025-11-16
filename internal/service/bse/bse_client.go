package bse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"time"

	"concall-analyser/internal/domain"
	"concall-analyser/internal/infrastructure/http"
)

// BSEClient defines the interface for BSE API operations
type BSEClient interface {
	FetchAnnouncements(ctx context.Context, fromDate, toDate time.Time) ([]domain.Announcement, error)
}

type bseClient struct {
	httpClient http.Client
}

// NewBSEClient creates a new BSE API client
func NewBSEClient(httpClient http.Client) BSEClient {
	return &bseClient{
		httpClient: httpClient,
	}
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

func (c *bseClient) FetchAnnouncements(ctx context.Context, fromDate, toDate time.Time) ([]domain.Announcement, error) {
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
	q.Set("pageno", "1")
	u.RawQuery = q.Encode()

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://www.bseindia.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Sec-CH-UA", `"Google Chrome";v="141", "Not?A_Brand";v="8", "Chromium";v="141"`)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", `"macOS"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch announcements: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != nethttp.StatusOK {
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

	return ar.Table, nil
}

