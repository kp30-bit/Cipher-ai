package analytics

import (
	"context"
	"fmt"
	"time"

	"concall-analyser/internal/domain"

	"github.com/google/uuid"
)

// AnalyticsService handles analytics business logic
type AnalyticsService interface {
	GetOrCreateSessionID(existingSessionID string) string
	RecordPageView(ctx context.Context, sessionID, endpoint string) error
	RecordAPICall(ctx context.Context, sessionID, endpoint string) error
	GetSummary(ctx context.Context) (*domain.AnalyticsSummary, error)
}

type analyticsService struct {
	repo domain.AnalyticsRepository
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(repo domain.AnalyticsRepository) AnalyticsService {
	return &analyticsService{
		repo: repo,
	}
}

// GetOrCreateSessionID returns existing session ID or generates a new one
func (s *analyticsService) GetOrCreateSessionID(existingSessionID string) string {
	if existingSessionID != "" {
		return existingSessionID
	}
	return uuid.New().String()
}

// RecordPageView records a page view event
func (s *analyticsService) RecordPageView(ctx context.Context, sessionID, endpoint string) error {
	event := &domain.AnalyticsEvent{
		SessionID: sessionID,
		Endpoint:  endpoint,
		EventType: "page_view",
		Timestamp: time.Now(),
	}
	
	return s.repo.RecordEvent(ctx, event)
}

// RecordAPICall records an API call event
func (s *analyticsService) RecordAPICall(ctx context.Context, sessionID, endpoint string) error {
	event := &domain.AnalyticsEvent{
		SessionID: sessionID,
		Endpoint:  endpoint,
		EventType: "api_call",
		Timestamp: time.Now(),
	}
	
	return s.repo.RecordEvent(ctx, event)
}

// GetSummary retrieves analytics summary
func (s *analyticsService) GetSummary(ctx context.Context) (*domain.AnalyticsSummary, error) {
	summary, err := s.repo.GetSummary(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get analytics summary: %w", err)
	}
	return summary, nil
}

