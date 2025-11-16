package mongo

import (
	"context"
	"fmt"
	"time"

	"concall-analyser/internal/db"
	"concall-analyser/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type analyticsRepository struct {
	coll *mongo.Collection
}

// NewAnalyticsRepository creates a new MongoDB implementation of AnalyticsRepository
func NewAnalyticsRepository(db *db.MongoDB) domain.AnalyticsRepository {
	repo := &analyticsRepository{
		coll: db.Collection("analytics"),
	}

	// Create indexes for better query performance
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "session_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "endpoint", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "timestamp", Value: -1}},
		},
	}

	_, _ = repo.coll.Indexes().CreateMany(ctx, indexes)

	return repo
}

func (r *analyticsRepository) RecordEvent(ctx context.Context, event *domain.AnalyticsEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	_, err := r.coll.InsertOne(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to insert analytics event: %w", err)
	}

	return nil
}

func (r *analyticsRepository) GetSummary(ctx context.Context) (*domain.AnalyticsSummary, error) {
	// Get total visits (only /api/list_concalls hits)
	totalVisits, err := r.coll.CountDocuments(ctx, bson.M{"endpoint": "/api/list_concalls"})
	if err != nil {
		return nil, fmt.Errorf("failed to count total visits: %w", err)
	}

	// Get unique users count
	uniqueUsers, err := r.GetUniqueUsersCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get unique users count: %w", err)
	}

	// Get API hits (events with event_type "api_call")
	apiHits, err := r.coll.CountDocuments(ctx, bson.M{"event_type": "api_call"})
	if err != nil {
		return nil, fmt.Errorf("failed to count API hits: %w", err)
	}

	// Get endpoint stats
	endpointStats, err := r.GetEndpointStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint stats: %w", err)
	}

	return &domain.AnalyticsSummary{
		TotalVisits:   totalVisits,
		UniqueUsers:   uniqueUsers,
		APIHits:       apiHits,
		EndpointStats: endpointStats,
		LastUpdated:   time.Now(),
	}, nil
}

func (r *analyticsRepository) GetUniqueUsersCount(ctx context.Context) (int64, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id": "$session_id",
			},
		},
		{
			"$count": "unique_sessions",
		},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate unique users: %w", err)
	}
	defer cursor.Close(ctx)

	var result struct {
		UniqueSessions int64 `bson:"unique_sessions"`
	}

	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("failed to decode unique users result: %w", err)
		}
		return result.UniqueSessions, nil
	}

	return 0, nil
}

func (r *analyticsRepository) GetEndpointStats(ctx context.Context) (map[string]int64, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$endpoint",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate endpoint stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := make(map[string]int64)
	for cursor.Next(ctx) {
		var result struct {
			Endpoint string `bson:"_id"`
			Count    int64  `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		stats[result.Endpoint] = result.Count
	}

	return stats, nil
}
