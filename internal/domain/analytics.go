package domain

import (
	"context"
	"time"
)

// AnalyticsEvent represents a single analytics event
type AnalyticsEvent struct {
	SessionID string    `bson:"session_id"`
	Endpoint  string    `bson:"endpoint"`
	EventType string    `bson:"event_type"` // "page_view" or "api_call"
	Timestamp time.Time `bson:"timestamp"`
}

// AnalyticsSummary represents aggregated analytics data
type AnalyticsSummary struct {
	TotalVisits   int64                  `json:"total_visits"`
	UniqueUsers   int64                  `json:"unique_users"`
	APIHits       int64                  `json:"api_hits"`
	EndpointStats map[string]int64       `json:"endpoint_stats"`
	LastUpdated   time.Time              `json:"last_updated"`
}

// AnalyticsRepository defines the interface for analytics data persistence
type AnalyticsRepository interface {
	// RecordEvent records a new analytics event
	RecordEvent(ctx context.Context, event *AnalyticsEvent) error
	
	// GetSummary returns aggregated analytics summary
	GetSummary(ctx context.Context) (*AnalyticsSummary, error)
	
	// GetUniqueUsersCount returns the count of unique sessions
	GetUniqueUsersCount(ctx context.Context) (int64, error)
	
	// GetEndpointStats returns hit counts per endpoint
	GetEndpointStats(ctx context.Context) (map[string]int64, error)
}

