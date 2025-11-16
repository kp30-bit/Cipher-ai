package controller

import (
	"concall-analyser/internal/interfaces"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine, u interfaces.Usecase) {
	// Prefix all API routes with /api
	api := r.Group("/api")
	{
		api.GET("/fetch_concalls", u.FetchConcallDataHandler)
		api.GET("/list_concalls", u.ListConcallHandler)
		api.GET("/find_concalls", u.FindConcallHandler)
		api.DELETE("/cleanup_concalls", u.CleanupConcallHandler)
	}
}
