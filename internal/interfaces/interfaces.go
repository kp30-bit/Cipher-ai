package interfaces

import (
	"github.com/gin-gonic/gin"
)

type Usecase interface {
	FetchConcallDataHandler(c *gin.Context)
	ListConcallHandler(c *gin.Context)
	FindConcallHandler(c *gin.Context)
	CleanupConcallHandler(c *gin.Context)
}
