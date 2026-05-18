// backend/internal/handler/repo_handler.go
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/autorestic/autorestic/internal/model"
	"github.com/autorestic/autorestic/internal/service"
	"github.com/gin-gonic/gin"
)

type RepoHandler struct {
	svc *service.RepoService
}

func NewRepoHandler(svc *service.RepoService) *RepoHandler {
	return &RepoHandler{svc: svc}
}

func (h *RepoHandler) Register(rg *gin.RouterGroup) {
	r := rg.Group("/repos")
	r.GET("", h.List)
	r.POST("", h.Create)
	r.GET("/check", h.CheckPathQueryDisabled)
	r.POST("/check", h.CheckPath)
	r.POST("/validate-access", h.ValidateAccess)
	r.GET("/:id", h.Get)
	r.PUT("/:id", h.Update)
	r.DELETE("/:id", h.Delete)
	r.POST("/:id/init", h.Init)
	r.POST("/:id/check", h.Check)
	r.POST("/:id/unlock", h.Unlock)
	r.POST("/:id/prune", h.Prune)
	r.GET("/:id/stats", h.Stats)
	r.GET("/:id/keys", h.ListKeys)
	r.POST("/:id/sync", h.Sync)
	r.GET("/:id/sync-state", h.SyncState)
}

func (h *RepoHandler) List(c *gin.Context) {
	repos, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, repos)
}

func (h *RepoHandler) Create(c *gin.Context) {
	var req model.CreateRepoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.svc.Create(req)
	if err != nil {
		if errors.Is(err, service.ErrRepositoryAccessInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrRepositoryAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrRepositoryLocked) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "locked": true})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := gin.H{"id": result.ID}
	if result.ImportStatus != "" {
		resp["import_status"] = result.ImportStatus
		resp["import_log_id"] = result.ImportLogID
	}
	if result.InitStatus != "" {
		resp["init_status"] = result.InitStatus
		if result.InitLogID > 0 {
			resp["init_log_id"] = result.InitLogID
		}
	}
	if result.UnlockLogID > 0 {
		resp["unlock_log_id"] = result.UnlockLogID
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *RepoHandler) ValidateAccess(c *gin.Context) {
	var req model.RepositoryAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ValidateRepoAccess(req); err != nil {
		if errors.Is(err, service.ErrRepositoryAccessInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrRepositoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RepoHandler) CheckPath(c *gin.Context) {
	var req model.RepositoryAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	probe, err := h.svc.CheckRepoPath(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrRepositoryAccessInvalid) {
			c.JSON(http.StatusOK, gin.H{"exists": true, "accessible": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"exists": probe.Exists, "accessible": probe.Accessible, "locked": probe.Locked})
}

func (h *RepoHandler) CheckPathQueryDisabled(c *gin.Context) {
	c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "repository access checks require POST /api/v1/repos/check"})
}

func (h *RepoHandler) Get(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	repo, err := h.svc.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, repo)
}

func (h *RepoHandler) Update(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req model.UpdateRepoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Update(id, req); err != nil {
		if errors.Is(err, service.ErrRepositoryAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RepoHandler) Delete(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *RepoHandler) Init(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.InitRepoAsync(id)
		if err != nil {
			h.respondRepoError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}
	result, err := h.svc.InitRepo(c.Request.Context(), id)
	if err != nil {
		h.respondRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr})
}

func (h *RepoHandler) Check(c *gin.Context) {
	// ID check - check existing repo
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.CheckRepoAsync(id)
		if err != nil {
			h.respondRepoError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}
	result, err := h.svc.CheckRepo(c.Request.Context(), id)
	if err != nil {
		h.respondRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr})
}

func (h *RepoHandler) Unlock(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.UnlockRepoAsync(id)
		if err != nil {
			h.respondRepoError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}
	result, err := h.svc.UnlockRepo(c.Request.Context(), id)
	if err != nil {
		h.respondRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr})
}

func (h *RepoHandler) Prune(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.PruneRepoAsync(id)
		if err != nil {
			h.respondRepoError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}
	result, err := h.svc.PruneRepo(c.Request.Context(), id)
	if err != nil {
		h.respondRepoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr})
}

func (h *RepoHandler) respondRepoError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrRepositoryAccessInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrRepositoryAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, service.ErrRepositoryLocked):
		status = http.StatusConflict
	case isValidationError(err):
		status = http.StatusBadRequest
	}
	resp := gin.H{"error": err.Error()}
	if errors.Is(err, service.ErrRepositoryLocked) {
		resp["locked"] = true
	}
	c.JSON(status, resp)
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "required") ||
		strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "parse ") ||
		strings.Contains(lower, "invalid ")
}

func (h *RepoHandler) Stats(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	result, err := h.svc.Cache().GetStatsView(id, c.Query("refresh") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *RepoHandler) ListKeys(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	result, err := h.svc.GetKeysCached(c.Request.Context(), id, c.Query("refresh") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *RepoHandler) Sync(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	domain := c.DefaultQuery("domain", "all")
	logID, err := h.svc.Cache().QueueSync(id, domain)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
}

func (h *RepoHandler) SyncState(c *gin.Context) {
	id, err := parseInt64Param(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	domains, err := h.svc.Cache().GetSyncStateDomains(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	states, err := h.svc.Cache().GetSyncStates(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"domains": domains,
		"states":  states,
	})
}
