package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"Finova/internal/market"
)

// NewRouter builds the HTTP router. The market handler, when non-nil, mounts the
// crawl/collection endpoints under /api/v1/market.
func NewRouter(db *pgxpool.Pool, marketH *market.Handler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api/v1")
	api.GET("/health", func(c *gin.Context) {
		if err := db.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	if marketH != nil {
		marketH.RegisterRoutes(api)
	}
	return r
}
