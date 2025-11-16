package domain

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConcallRepository defines the interface for concall data persistence
type ConcallRepository interface {
	// FindExistingNames finds all existing names from the given list
	FindExistingNames(ctx context.Context, names []string) (map[string]bool, error)
	
	// InsertMany inserts multiple concall summaries
	InsertMany(ctx context.Context, summaries []ConcallSummary) error
	
	// FindWithFilter finds documents matching the filter with options
	FindWithFilter(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]ConcallLite, error)
	
	// CountDocuments counts documents matching the filter
	CountDocuments(ctx context.Context, filter bson.M) (int64, error)
	
	// DeleteMany deletes documents matching the filter
	DeleteMany(ctx context.Context, filter bson.M) (int64, error)
	
	// Aggregate performs an aggregation pipeline
	Aggregate(ctx context.Context, pipeline []bson.M) (*mongo.Cursor, error)
}

