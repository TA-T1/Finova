package market

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler exposes the market module over HTTP.
type Handler struct {
	col  *Collector
	repo *Repository
}

// NewHandler builds a Handler from a collector and repository.
func NewHandler(col *Collector, repo *Repository) *Handler {
	return &Handler{col: col, repo: repo}
}

// RegisterRoutes mounts the market endpoints under rg (e.g. /api/v1).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	m := rg.Group("/market")
	m.POST("/collect/options", h.collectOptions)
	m.POST("/collect/daily/:insCode", h.collectDaily)
	m.GET("/options", h.listOptions)
	m.GET("/underlyings", h.listUnderlyings)
}

// collectOptions triggers one option-chain collection pass.
func (h *Handler) collectOptions(c *gin.Context) {
	res, err := h.col.CollectOptionChain(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// collectDaily backfills the daily price history of an underlying instrument.
// Pass ?adjusted=true to request a split/dividend-adjusted series.
func (h *Handler) collectDaily(c *gin.Context) {
	insCode := c.Param("insCode")
	adjusted, _ := strconv.ParseBool(c.DefaultQuery("adjusted", "false"))
	n, err := h.col.BackfillDaily(c.Request.Context(), insCode, adjusted)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"insCode": insCode, "adjusted": adjusted, "days": n})
}

// listOptions returns stored contracts joined with their latest quote.
func (h *Handler) listOptions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))
	rows, err := h.repo.ListOptionChain(c.Request.Context(), ChainFilter{
		UnderlyingSymbol: c.Query("underlying"),
		OptionType:       c.Query("type"),
		Limit:            limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(rows), "items": rows})
}

// listUnderlyings returns all known underlyings.
func (h *Handler) listUnderlyings(c *gin.Context) {
	rows, err := h.repo.ListUnderlyings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(rows), "items": rows})
}
