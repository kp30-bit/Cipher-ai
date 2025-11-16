package usecase

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (cf *concallFetcher) GetAnalyticsHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := cf.analyticsService.GetSummary(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch analytics",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

