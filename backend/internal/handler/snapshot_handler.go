// backend/internal/handler/snapshot_handler.go
package handler

import (
	"net/http"

	"github.com/autorestic/autorestic/internal/service"
	"github.com/gin-gonic/gin"
)

type SnapshotHandler struct {
	svc *service.SnapshotService
}

func NewSnapshotHandler(svc *service.SnapshotService) *SnapshotHandler {
	return &SnapshotHandler{svc: svc}
}

func (h *SnapshotHandler) Register(rg *gin.RouterGroup) {
	// Use /snapshots route with repo_id as query param to avoid conflict with /repos/:id
	r := rg.Group("/snapshots")
	r.GET("", h.List)
	r.GET("/files", h.ListFiles)
	r.GET("/diff", h.Diff)
	r.POST("/restore", h.Restore)
	r.DELETE("", h.Forget)
	r.POST("/find", h.Find)
}

func (h *SnapshotHandler) getRepoID(c *gin.Context) (int64, bool) {
	repoID, err := parseInt64Query(c, "repo_id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid repo_id required"})
		return 0, false
	}
	return repoID, true
}

func (h *SnapshotHandler) List(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	page, _ := parseIntQueryWithDefault(c, "page", 1)
	pageSize, _ := parseIntQueryWithDefault(c, "page_size", 50)
	snaps, err := h.svc.ListSnapshotIndexFiltered(repoID, page, pageSize, c.Query("refresh") == "true", c.Query("update_filter"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, snaps)
}

func (h *SnapshotHandler) ListFiles(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	snapID := c.Query("snapshot_id")
	if snapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "snapshot_id required"})
		return
	}
	files, err := h.svc.ListFilesView(repoID, snapID, c.Query("path"), c.Query("refresh") == "true")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, files)
}

func (h *SnapshotHandler) Diff(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	snap1 := c.Query("snap1")
	snap2 := c.Query("snap2")
	if snap1 == "" || snap2 == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "snap1 and snap2 required"})
		return
	}
	diff, err := h.svc.Diff(c.Request.Context(), repoID, snap1, snap2)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"diff": diff})
}

func (h *SnapshotHandler) Restore(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	snapID := c.Query("snapshot_id")
	if snapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "snapshot_id required"})
		return
	}

	var req struct {
		TargetPath string   `json:"target_path" binding:"required"`
		Includes   []string `json:"includes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.RestoreAsync(repoID, snapID, req.TargetPath, req.Includes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}

	result, err := h.svc.Restore(c.Request.Context(), repoID, snapID, req.TargetPath, req.Includes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode})
}

func (h *SnapshotHandler) Forget(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	snapID := c.Query("snapshot_id")
	if snapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "snapshot_id required"})
		return
	}
	if wantsAsync(c) {
		logID, err := h.svc.ForgetAsync(repoID, snapID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"log_id": logID, "status": "running"})
		return
	}
	result, err := h.svc.Forget(c.Request.Context(), repoID, snapID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"log_id": result.LogID, "exit_code": result.ExitCode})
}

func (h *SnapshotHandler) Find(c *gin.Context) {
	repoID, ok := h.getRepoID(c)
	if !ok {
		return
	}
	pattern := c.Query("pattern")
	if pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pattern required"})
		return
	}
	result, err := h.svc.Find(c.Request.Context(), repoID, pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
