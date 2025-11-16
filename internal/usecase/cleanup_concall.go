package usecase

import (
	"context"
	"log"
	"net/http"
	"time"

	"concall-analyser/internal/domain"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DuplicateGroup struct {
	Name  string                  `bson:"_id"`
	Docs  []domain.ConcallSummary `bson:"docs"`
	Count int                     `bson:"count"`
}

func (cf *concallFetcher) CleanupConcallHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	// Step 1: Delete all records with guidance == "NA"
	naFilter := bson.M{"guidance": "NA"}
	naDeletedCount, err := cf.repo.DeleteMany(ctx, naFilter)
	if err != nil {
		log.Printf("❌ Failed to delete NA guidance records: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to delete NA guidance records",
			"details": err.Error(),
		})
		return
	}
	log.Printf("🗑️ Deleted %d records with guidance='NA'", naDeletedCount)

	// Step 2: Find and delete duplicates based on name field
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
				"count": bson.M{"$gt": 1},
			},
		},
	}

	cursor, err := cf.repo.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("❌ Failed to find duplicates: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to find duplicates",
			"details": err.Error(),
		})
		return
	}
	defer cursor.Close(ctx)

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

	for _, group := range duplicateGroups {
		if len(group.Docs) <= 1 {
			continue
		}

		var keepID primitive.ObjectID
		var latestTime time.Time

		for _, doc := range group.Docs {
			if doc.CreatedAt.After(latestTime) || latestTime.IsZero() {
				latestTime = doc.CreatedAt
				keepID = doc.ID
			}
		}

		deleteFilter := bson.M{
			"name": group.Name,
			"_id":  bson.M{"$ne": keepID},
		}

		deleted, err := cf.repo.DeleteMany(ctx, deleteFilter)
		if err != nil {
			log.Printf("⚠️ Failed to delete duplicates for name '%s': %v", group.Name, err)
			continue
		}

		duplicateDeletedCount += deleted
		duplicateNamesProcessed++
		log.Printf("🗑️ Deleted %d duplicate(s) for name '%s' (kept most recent)", deleted, group.Name)
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

