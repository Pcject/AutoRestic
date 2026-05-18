// backend/internal/handler/settings_handler.go
package handler

import (
	"net/http"

	"github.com/autorestic/autorestic/internal/config"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	cfg *config.Config
}

func NewSettingsHandler(cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{cfg: cfg}
}

func (h *SettingsHandler) Register(rg *gin.RouterGroup) {
	r := rg.Group("/settings")
	r.GET("", h.Get)
}

func (h *SettingsHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"restic_bin":      h.cfg.ResticBin,
		"db_path":         h.cfg.DBPath,
		"log_retain_days": h.cfg.LogRetainDays,
		"port":            h.cfg.Port,
		"auth_enabled":    h.cfg.AuthToken != "",
		"cors_origins":    h.cfg.CORSOrigins,
	})
}
