package usecase

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (cf *concallFetcher) FindConcallHandler(c *gin.Context) {
	rawName := c.Query("name")
	if strings.TrimSpace(rawName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'name' is required"})
		return
	}

	name := strings.TrimSpace(strings.ReplaceAll(rawName, "+", " "))

	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "12")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 12
	}
	skip := int64((page - 1) * limit)
	limit64 := int64(limit)

	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()

	escaped := regexp.QuoteMeta(name)
	filter := bson.M{
		"name": bson.M{
			"$regex":   escaped,
			"$options": "i",
		},
	}

	projection := bson.M{
		"name":     1,
		"date":     1,
		"guidance": 1,
		"_id":      0,
	}

	findOpts := options.Find().
		SetProjection(projection).
		SetSort(bson.D{{Key: "date", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit64)

	totalCount, err := cf.repo.CountDocuments(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count documents", "details": err.Error()})
		return
	}

	results, err := cf.repo.FindWithFilter(ctx, filter, findOpts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query MongoDB", "details": err.Error()})
		return
	}

	totalPages := (totalCount + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"meta": gin.H{
			"query":      name,
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": totalPages,
		},
		"data": results,
	})
}

