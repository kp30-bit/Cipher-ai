package controller

import (
	"concall-analyser/internal/interfaces"
	"concall-analyser/internal/middleware"
	"concall-analyser/internal/service/analytics"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, u interfaces.Usecase, analyticsService analytics.AnalyticsService) {
	// Add analytics middleware to track all requests
	r.Use(middleware.AnalyticsMiddleware(analyticsService))

	// Prefix all API routes with /api
	api := r.Group("/api")
	{
		api.GET("/fetch_concalls", u.FetchConcallDataHandler)
		api.GET("/list_concalls", u.ListConcallHandler)
		api.GET("/find_concalls", u.FindConcallHandler)
		api.DELETE("/cleanup_concalls", u.CleanupConcallHandler)
		api.GET("/analytics", u.GetAnalyticsHandler)
	}
}
