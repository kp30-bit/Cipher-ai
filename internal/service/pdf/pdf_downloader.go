package pdf

import (
	"context"
	"fmt"
	"io"
	nethttp "net/http"
	"os"
	"path/filepath"

	"concall-analyser/internal/infrastructure/file"
	"concall-analyser/internal/infrastructure/http"
)

// PDFDownloader defines the interface for PDF download operations
type PDFDownloader interface {
	Download(ctx context.Context, attachmentName, destDir, saveAs string) (string, error)
}

type pdfDownloader struct {
	httpClient http.Client
}

// NewPDFDownloader creates a new PDF downloader service
func NewPDFDownloader(httpClient http.Client) PDFDownloader {
	return &pdfDownloader{
		httpClient: httpClient,
	}
}

func (d *pdfDownloader) Download(ctx context.Context, attachmentName, destDir, saveAs string) (string, error) {
	if attachmentName == "" {
		return "", fmt.Errorf("attachment name is empty")
	}

	baseURL := "https://www.bseindia.com/xml-data/corpfiling/AttachLive/"
	fullURL := baseURL + attachmentName

	req, err := nethttp.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.bseindia.com/")
	req.Header.Set("Accept", "application/pdf")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download PDF: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download %s, status %d", attachmentName, resp.StatusCode)
	}

	if err := file.CreateDirectory(destDir); err != nil {
		return "", err
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

