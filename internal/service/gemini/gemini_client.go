package gemini

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// GeminiClient defines the interface for Gemini AI operations
type GeminiClient interface {
	SummarizePDF(ctx context.Context, pdfPath string) (string, error)
	Close() error
}

type geminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

// NewGeminiClient creates a new Gemini AI client
func NewGeminiClient(ctx context.Context, apiKey string) (GeminiClient, error) {
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	model := genaiClient.GenerativeModel("gemini-2.5-flash")
	return &geminiClient{
		client: genaiClient,
		model:  model,
	}, nil
}

func (g *geminiClient) Close() error {
	return g.client.Close()
}

func (g *geminiClient) SummarizePDF(ctx context.Context, pdfPath string) (string, error) {
	// Upload file
	file, err := g.client.UploadFileFromPath(ctx, pdfPath, &genai.UploadFileOptions{
		MIMEType: "application/pdf",
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload PDF: %w", err)
	}

	fmt.Printf("✅ Uploaded file: %s (MIME: %s)\n", file.Name, file.MIMEType)

	prompt := `Go through the concall and identify if management has given any guidance for fy26 on the future growth of the company in terms of revenue, earnings, eps etc. If yes, then just return the fy26' guidance after quantifying it and return nothing else. If no guidance is provided, then return "NA". Your responsse should be just 1 line providing the guidance for fy26' in numbers otherwise NA.`

	resp, err := g.makeCallWithRetry(ctx, file, prompt)
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
	if err := g.client.DeleteFile(ctx, file.Name); err != nil {
		log.Printf("Warning: failed to delete uploaded file %s: %v", file.Name, err)
	}

	return strings.TrimSpace(output.String()), nil
}

func (g *geminiClient) makeCallWithRetry(ctx context.Context, file *genai.File, prompt string) (*genai.GenerateContentResponse, error) {
	const maxRetries = 5
	baseDelay := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		resp, err := g.model.GenerateContent(ctx,
			genai.FileData{MIMEType: file.MIMEType, URI: file.URI},
			genai.Text(prompt),
		)

		if err == nil {
			return resp, nil
		}

		if !isRetriableError(err) {
			return nil, fmt.Errorf("Gemini generation failed with non-retriable error: %w", err)
		}

		delay := baseDelay * time.Duration(1<<i)
		jitter := time.Duration(rand.Int63n(int64(delay) / 5))
		sleepTime := delay + jitter

		log.Printf("⚠️ Rate limit or transient error detected. Retrying in %v (Attempt %d/%d). Error: %v", sleepTime, i+1, maxRetries, err)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepTime):
		}
	}

	return nil, fmt.Errorf("Gemini generation failed after %d retries due to rate limits/transient errors", maxRetries)
}

func isRetriableError(err error) bool {
	if apiErr, ok := err.(*googleapi.Error); ok {
		return apiErr.Code == 429 || apiErr.Code == 500 || apiErr.Code == 503
	}
	return false
}

