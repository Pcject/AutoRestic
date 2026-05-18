package model

import "time"

type BackupTask struct {
	ID            int64      `json:"id"`
	RepoID        int64      `json:"repo_id"`
	Name          string     `json:"name"`
	SourcePaths   string     `json:"source_paths"`   // JSON array
	Excludes      string     `json:"excludes"`       // JSON array
	Tags          string     `json:"tags"`           // JSON array
	CronExpr      string     `json:"cron_expr"`      // Empty = no schedule
	CronEnabled   bool       `json:"cron_enabled"`
	ForgetPolicy  string     `json:"forget_policy"`  // JSON: keep-last, keep-daily, etc.
	PreHooks      string     `json:"pre_hooks"`      // JSON array
	PostHooks     string     `json:"post_hooks"`     // JSON array
	ExtraFlags    string     `json:"extra_flags"`    // JSON object
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastRunAt     *time.Time `json:"last_run_at"`
	NextRunAt     *time.Time `json:"next_run_at"`
}

type CreateTaskRequest struct {
	RepoID       int64  `json:"repo_id" binding:"required"`
	Name         string `json:"name" binding:"required"`
	SourcePaths  string `json:"source_paths" binding:"required"` // JSON array string
	Excludes     string `json:"excludes,omitempty"`
	Tags         string `json:"tags,omitempty"`
	CronExpr     string `json:"cron_expr,omitempty"`
	CronEnabled  bool   `json:"cron_enabled"`
	ForgetPolicy string `json:"forget_policy,omitempty"`
	PreHooks     string `json:"pre_hooks,omitempty"`
	PostHooks    string `json:"post_hooks,omitempty"`
	ExtraFlags   string `json:"extra_flags,omitempty"`
}

type UpdateTaskRequest struct {
	Name         *string `json:"name,omitempty"`
	SourcePaths  *string `json:"source_paths,omitempty"`
	Excludes     *string `json:"excludes,omitempty"`
	Tags         *string `json:"tags,omitempty"`
	CronExpr     *string `json:"cron_expr,omitempty"`
	CronEnabled  *bool   `json:"cron_enabled,omitempty"`
	ForgetPolicy *string `json:"forget_policy,omitempty"`
	PreHooks     *string `json:"pre_hooks,omitempty"`
	PostHooks    *string `json:"post_hooks,omitempty"`
	ExtraFlags   *string `json:"extra_flags,omitempty"`
}

type TaskQuery struct {
	RepoID      *int64 `form:"repo_id"`
	CronEnabled *bool  `form:"cron_enabled"`
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=50"`
}