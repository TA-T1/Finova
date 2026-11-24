package market

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	m.GET("/status", h.marketStatus)
	m.POST("/collect/options", h.collectOptions)
	m.POST("/collect/daily/:insCode", h.collectDaily)
	m.GET("/options", h.listOptions)
	m.GET("/underlyings", h.listUnderlyings)
	m.GET("/chain", h.optionChain)
	m.GET("/symbol/:symbol", h.symbolPage)
}

// marketStatus reports the current Tehran trading session state and the last
// quote timestamp persisted by the collector.
func (h *Handler) marketStatus(c *gin.Context) {
	lastUpdate, err := h.repo.LastMarketUpdate(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"marketOpen": MarketOpenAt(time.Now()),
		"lastUpdate": lastUpdate,
		"timezone":   tehranTimezone,
	})
}

// collectOptions triggers one option-chain collection pass.
func (h *Handler) collectOptions(c *gin.Context) {
	res, err := h.col.CollectOptionChain(c.Request.Context())
	if err != nil {
		if errors.Is(err, ErrMarketClosed) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
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
		UnderlyingKind:   c.Query("underlyingKind"),
		OptionType:       c.Query("type"),
		Limit:            limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(rows), "items": rows})
}

// listUnderlyings returns known underlyings, optionally filtered by ?kind=.
func (h *Handler) listUnderlyings(c *gin.Context) {
	rows, err := h.repo.ListUnderlyings(c.Request.Context(), c.Query("kind"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(rows), "items": rows})
}

// optionChain returns call/put pairs for an underlying and Gregorian expiry.
func (h *Handler) optionChain(c *gin.Context) {
	symbol := strings.TrimSpace(c.Query("underlying"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "underlying is required"})
		return
	}
	expiryText := c.Query("expiry")
	expiry, err := time.Parse("2006-01-02", expiryText)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expiry must use YYYY-MM-DD"})
		return
	}
	strikes, err := h.repo.ListOptionChainForExpiry(c.Request.Context(), symbol, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(strikes) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active option contracts for underlying and expiry"})
		return
	}
	c.JSON(http.StatusOK, OptionChain{
		UnderlyingSymbol: symbol,
		ExpiryDate:       expiryText,
		Strikes:          strikes,
	})
}

// symbolPage returns the underlying quote and its active options summary.
func (h *Handler) symbolPage(c *gin.Context) {
	symbol := strings.TrimSpace(c.Param("symbol"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol is required"})
		return
	}
	page, err := h.repo.GetSymbolPage(c.Request.Context(), symbol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if page == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "symbol has no active option contracts"})
		return
	}
	c.JSON(http.StatusOK, page)
}
