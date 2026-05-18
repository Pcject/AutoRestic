// backend/internal/handler/log_handler.go
package handler

import (
	"net/http"
	"strconv"

	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/service"
	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	svc *service.LogService
}

func NewLogHandler(svc *service.LogService) *LogHandler {
	return &LogHandler{svc: svc}
}

func (h *LogHandler) Register(rg *gin.RouterGroup) {
	r := rg.Group("/logs")
	r.GET("", h.Query)
	r.GET("/:id", h.Get)
	r.POST("/:id/cancel", h.Cancel)
}

func (h *LogHandler) Query(c *gin.Context) {
	var q model.LogQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.Query(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *LogHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	outputLimit, err := parseIntQueryWithDefault(c, "limit", model.DefaultLogOutputLimit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logEntry, err := h.svc.GetByID(id, outputLimit)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, logEntry)
}

func (h *LogHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.Cancel(id); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
