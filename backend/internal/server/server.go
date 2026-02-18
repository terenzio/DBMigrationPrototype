package server

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/terence/dbmigration-backend/internal/handler"
)

func Setup(psHandler *handler.ProductSuiteHandler, db *sql.DB) *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		if err := db.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	api := r.Group("/api/v1")
	{
		api.POST("/product-suites", psHandler.Create)
	}

	return r
}
