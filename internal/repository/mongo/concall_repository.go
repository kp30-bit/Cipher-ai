package mongo

import (
	"context"
	"fmt"

	"concall-analyser/internal/db"
	"concall-analyser/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type concallRepository struct {
	coll *mongo.Collection
}

// NewConcallRepository creates a new MongoDB implementation of ConcallRepository
func NewConcallRepository(db *db.MongoDB) domain.ConcallRepository {
	return &concallRepository{
		coll: db.Collection("guidances"),
	}
}

func (r *concallRepository) FindExistingNames(ctx context.Context, names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return make(map[string]bool), nil
	}

	filter := bson.M{"name": bson.M{"$in": names}}
	cursor, err := r.coll.Find(ctx, filter)
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

	return existingNames, nil
}

func (r *concallRepository) InsertMany(ctx context.Context, summaries []domain.ConcallSummary) error {
	if len(summaries) == 0 {
		return nil
	}

	docs := make([]interface{}, len(summaries))
	for i, summary := range summaries {
		docs[i] = summary
	}

	_, err := r.coll.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert summaries: %w", err)
	}

	return nil
}

func (r *concallRepository) FindWithFilter(ctx context.Context, filter bson.M, opts *options.FindOptions) ([]domain.ConcallLite, error) {
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query MongoDB: %w", err)
	}
	defer cursor.Close(ctx)

	var results []domain.ConcallLite
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode documents: %w", err)
	}

	return results, nil
}

func (r *concallRepository) CountDocuments(ctx context.Context, filter bson.M) (int64, error) {
	count, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count documents: %w", err)
	}
	return count, nil
}

func (r *concallRepository) DeleteMany(ctx context.Context, filter bson.M) (int64, error) {
	result, err := r.coll.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete documents: %w", err)
	}
	return result.DeletedCount, nil
}

func (r *concallRepository) Aggregate(ctx context.Context, pipeline []bson.M) (*mongo.Cursor, error) {
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate: %w", err)
	}
	return cursor, nil
}

