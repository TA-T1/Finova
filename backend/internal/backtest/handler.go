package backtest

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type RepositoryInterface interface {
	List(context.Context, ListQuery) ([]Summary, error)
	Get(context.Context, int) (*Detail, error)
}

type Handler struct {
	repo RepositoryInterface
}

func NewHandler(repo RepositoryInterface) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	routes := rg.Group("/backtests")
	routes.GET("", h.list)
	routes.GET("/:id", h.detail)
}

func ParseListQuery(values url.Values) (ListQuery, error) {
	get := func(key string) string {
		if items := values[key]; len(items) > 0 {
			return strings.TrimSpace(items[0])
		}
		return ""
	}
	limit := 100
	if raw := get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			return ListQuery{}, errBadLimit
		}
		limit = parsed
	}
	return ListQuery{
		Underlying: get("underlying"),
		Template:   get("template"),
		Model:      get("model"),
		Limit:      limit,
	}, nil
}

var errBadLimit = &badLimitError{}

type badLimitError struct{}

func (*badLimitError) Error() string {
	return "limit must be an integer between 1 and 500"
}

func (h *Handler) list(c *gin.Context) {
	query, err := ParseListQuery(c.Request.URL.Query())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, err := h.repo.List(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ListResponse{Count: len(items), Items: items})
}

func (h *Handler) detail(c *gin.Context) {
	raw := c.Param("id")
	id, err := strconv.Atoi(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be an integer"})
		return
	}
	item, err := h.repo.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backtest not found"})
		return
	}
	c.JSON(http.StatusOK, item)
}
