package model

import "time"

type Repository struct {
	ID                      int64      `json:"id"`
	Name                    string     `json:"name"`
	Type                    string     `json:"type"`
	Endpoint                string     `json:"endpoint"`
	PasswordEncrypted       string     `json:"-"`
	RcloneConfig            string     `json:"-"`
	RcloneConfigEncrypted   string     `json:"-"`
	HasRcloneConfig         bool       `json:"has_rclone_config"`
	WebdavURL               string     `json:"webdav_url,omitempty"`
	WebdavUser              string     `json:"webdav_user,omitempty"`
	WebdavPasswordEncrypted string     `json:"-"`
	Options                 string     `json:"options"`
	PruneEnabled            bool       `json:"prune_enabled"`
	PruneCronExpr           string     `json:"prune_cron_expr"`
	PruneArgs               string     `json:"prune_args"`
	CheckEnabled            bool       `json:"check_enabled"`
	CheckCronExpr           string     `json:"check_cron_expr"`
	CheckArgs               string     `json:"check_args"`
	SyncStatus              string     `json:"sync_status,omitempty"`
	LastSyncedAt            *time.Time `json:"last_synced_at,omitempty"`
	SnapshotCount           int        `json:"snapshot_count"`
	FileIndexStatus         string     `json:"file_index_status,omitempty"`
	LastCheckStatus         string     `json:"last_check_status,omitempty"`
	LastCheckAt             *time.Time `json:"last_check_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type CreateRepoRequest struct {
	Name           string `json:"name" binding:"required"`
	Type           string `json:"type" binding:"required,oneof=local rclone webdav"`
	Endpoint       string `json:"endpoint"`
	Password       string `json:"password" binding:"required"`
	RcloneConfig   string `json:"rclone_config,omitempty"`
	WebdavURL      string `json:"webdav_url,omitempty"`
	WebdavUser     string `json:"webdav_user,omitempty"`
	WebdavPassword string `json:"webdav_password,omitempty"`
	Options        string `json:"options,omitempty"`
	InitOnCreate   bool   `json:"init_on_create,omitempty"`
	AutoUnlock     bool   `json:"auto_unlock,omitempty"`
	PruneEnabled   *bool  `json:"prune_enabled,omitempty"`
	PruneCronExpr  string `json:"prune_cron_expr,omitempty"`
	PruneArgs      string `json:"prune_args,omitempty"`
	CheckEnabled   *bool  `json:"check_enabled,omitempty"`
	CheckCronExpr  string `json:"check_cron_expr,omitempty"`
	CheckArgs      string `json:"check_args,omitempty"`
}

type UpdateRepoRequest struct {
	Name           *string `json:"name,omitempty"`
	Endpoint       *string `json:"endpoint,omitempty"`
	Password       *string `json:"password,omitempty"`
	RcloneConfig   *string `json:"rclone_config,omitempty"`
	WebdavURL      *string `json:"webdav_url,omitempty"`
	WebdavUser     *string `json:"webdav_user,omitempty"`
	WebdavPassword *string `json:"webdav_password,omitempty"`
	Options        *string `json:"options,omitempty"`
	PruneEnabled   *bool   `json:"prune_enabled,omitempty"`
	PruneCronExpr  *string `json:"prune_cron_expr,omitempty"`
	PruneArgs      *string `json:"prune_args,omitempty"`
	CheckEnabled   *bool   `json:"check_enabled,omitempty"`
	CheckCronExpr  *string `json:"check_cron_expr,omitempty"`
	CheckArgs      *string `json:"check_args,omitempty"`
}

type RepositoryAccessRequest struct {
	Type           string `json:"type" binding:"required,oneof=local rclone webdav"`
	Endpoint       string `json:"endpoint"`
	Password       string `json:"password" binding:"required"`
	RcloneConfig   string `json:"rclone_config,omitempty"`
	WebdavURL      string `json:"webdav_url,omitempty"`
	WebdavUser     string `json:"webdav_user,omitempty"`
	WebdavPassword string `json:"webdav_password,omitempty"`
}
