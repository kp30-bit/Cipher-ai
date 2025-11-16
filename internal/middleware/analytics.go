package middleware

import (
	"context"
	"log"
	"strings"
	"time"

	"concall-analyser/internal/service/analytics"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "session_id"
	sessionMaxAge     = 30 * 24 * 60 * 60 // 30 days in seconds
)

// AnalyticsMiddleware creates middleware to track analytics events
func AnalyticsMiddleware(analyticsService analytics.AnalyticsService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get or create session ID from cookie
		sessionID, err := c.Cookie(sessionCookieName)
		if err != nil || sessionID == "" {
			sessionID = analyticsService.GetOrCreateSessionID("")
			// Set cookie for future requests
			c.SetCookie(sessionCookieName, sessionID, sessionMaxAge, "/", "", false, true)
		} else {
			// Validate and potentially refresh session ID
			sessionID = analyticsService.GetOrCreateSessionID(sessionID)
		}

		// Get endpoint path
		endpoint := c.Request.URL.Path

		// Determine event type based on endpoint
		isAPICall := strings.HasPrefix(endpoint, "/api/")
		isHomepage := endpoint == "/" || endpoint == "" || (!isAPICall && !strings.HasPrefix(endpoint, "/static"))

		// Continue with the request first
		c.Next()

		// Check response status - skip tracking 304 (Not Modified) responses
		statusCode := c.Writer.Status()
		if statusCode == 304 {
			return
		}

		// Record event asynchronously to avoid blocking the request
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if isAPICall {
				// Track API calls
				if err := analyticsService.RecordAPICall(ctx, sessionID, endpoint); err != nil {
					log.Printf("Failed to record API call analytics: %v", err)
				}
			} else if isHomepage {
				// Track homepage visits
				if err := analyticsService.RecordPageView(ctx, sessionID, "/"); err != nil {
					log.Printf("Failed to record page view analytics: %v", err)
				}
			}
		}()
	}
}

