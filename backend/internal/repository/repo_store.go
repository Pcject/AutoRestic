package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/autorestic/autorestic/internal/model"
)

type RepoStore struct {
	db *sql.DB
}

func NewRepoStore(db *sql.DB) *RepoStore {
	return &RepoStore{db: db}
}

func (s *RepoStore) DB() *sql.DB {
	return s.db
}

func (s *RepoStore) List() ([]model.Repository, error) {
	query := s.repoSelectQuery("ORDER BY created_at DESC")
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.Repository
	for rows.Next() {
		repo, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *RepoStore) GetByID(id int64) (*model.Repository, error) {
	row := s.db.QueryRow(s.repoSelectQuery("WHERE id = ?"), id)
	repo, err := scanRepository(row)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

func (s *RepoStore) Create(r *model.Repository) (int64, error) {
	query, args := s.repoInsertArgs(r)
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *RepoStore) Update(r *model.Repository) error {
	query, args := s.repoUpdateArgs(r)
	_, err := s.db.Exec(query, args...)
	return err
}

func (s *RepoStore) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM repositories WHERE id = ?", id)
	return err
}

type repositoryScanner interface {
	Scan(dest ...any) error
}

func scanRepository(scanner repositoryScanner) (model.Repository, error) {
	var repo model.Repository
	if err := scanner.Scan(
		&repo.ID, &repo.Name, &repo.Type, &repo.Endpoint,
		&repo.PasswordEncrypted, &repo.RcloneConfigEncrypted, &repo.HasRcloneConfig,
		&repo.WebdavURL, &repo.WebdavUser, &repo.WebdavPasswordEncrypted,
		&repo.Options, &repo.PruneEnabled, &repo.PruneCronExpr, &repo.PruneArgs,
		&repo.CheckEnabled, &repo.CheckCronExpr, &repo.CheckArgs,
		&repo.CreatedAt, &repo.UpdatedAt,
	); err != nil {
		return model.Repository{}, err
	}
	return repo, nil
}

func (s *RepoStore) repoSelectQuery(suffix string) string {
	if s.hasColumn("repositories", "rclone_config_encrypted") {
		return `SELECT id, name, type, endpoint, password_encrypted,
			COALESCE(NULLIF(rclone_config_encrypted, ''), '') AS rclone_config_encrypted,
			CASE WHEN COALESCE(NULLIF(rclone_config_encrypted, ''), NULLIF(rclone_config, '')) IS NOT NULL THEN 1 ELSE 0 END AS has_rclone_config,
			webdav_url, webdav_user, webdav_password_encrypted, options,
			prune_enabled, prune_cron_expr, prune_args,
			check_enabled, check_cron_expr, check_args,
			created_at, updated_at
		FROM repositories ` + suffix
	}

	return `SELECT id, name, type, endpoint, password_encrypted,
		COALESCE(NULLIF(rclone_config, ''), '') AS rclone_config_encrypted,
		CASE WHEN NULLIF(rclone_config, '') IS NOT NULL THEN 1 ELSE 0 END AS has_rclone_config,
		webdav_url, webdav_user, webdav_password_encrypted, options,
		prune_enabled, prune_cron_expr, prune_args,
		check_enabled, check_cron_expr, check_args,
		created_at, updated_at
	FROM repositories ` + suffix
}

func (s *RepoStore) repoInsertArgs(r *model.Repository) (string, []any) {
	if s.hasColumn("repositories", "rclone_config_encrypted") {
		return `INSERT INTO repositories (name, type, endpoint, password_encrypted,
			rclone_config, rclone_config_encrypted, webdav_url, webdav_user, webdav_password_encrypted, options,
			prune_enabled, prune_cron_expr, prune_args, check_enabled, check_cron_expr, check_args)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[]any{
				r.Name, r.Type, r.Endpoint, r.PasswordEncrypted,
				"", r.RcloneConfigEncrypted, r.WebdavURL, r.WebdavUser, r.WebdavPasswordEncrypted, r.Options,
				r.PruneEnabled, r.PruneCronExpr, r.PruneArgs, r.CheckEnabled, r.CheckCronExpr, r.CheckArgs,
			}
	}

	return `INSERT INTO repositories (name, type, endpoint, password_encrypted,
		rclone_config, webdav_url, webdav_user, webdav_password_encrypted, options,
		prune_enabled, prune_cron_expr, prune_args, check_enabled, check_cron_expr, check_args)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		[]any{
			r.Name, r.Type, r.Endpoint, r.PasswordEncrypted,
			r.RcloneConfigEncrypted, r.WebdavURL, r.WebdavUser, r.WebdavPasswordEncrypted, r.Options,
			r.PruneEnabled, r.PruneCronExpr, r.PruneArgs, r.CheckEnabled, r.CheckCronExpr, r.CheckArgs,
		}
}

func (s *RepoStore) repoUpdateArgs(r *model.Repository) (string, []any) {
	now := time.Now()
	if s.hasColumn("repositories", "rclone_config_encrypted") {
		return `UPDATE repositories SET name=?, endpoint=?, password_encrypted=?,
			rclone_config=?, rclone_config_encrypted=?, webdav_url=?, webdav_user=?, webdav_password_encrypted=?,
			options=?, prune_enabled=?, prune_cron_expr=?, prune_args=?,
			check_enabled=?, check_cron_expr=?, check_args=?, updated_at=?
		WHERE id=?`,
			[]any{
				r.Name, r.Endpoint, r.PasswordEncrypted,
				"", r.RcloneConfigEncrypted, r.WebdavURL, r.WebdavUser, r.WebdavPasswordEncrypted,
				r.Options, r.PruneEnabled, r.PruneCronExpr, r.PruneArgs,
				r.CheckEnabled, r.CheckCronExpr, r.CheckArgs, now, r.ID,
			}
	}

	return `UPDATE repositories SET name=?, endpoint=?, password_encrypted=?,
		rclone_config=?, webdav_url=?, webdav_user=?, webdav_password_encrypted=?,
		options=?, prune_enabled=?, prune_cron_expr=?, prune_args=?,
		check_enabled=?, check_cron_expr=?, check_args=?, updated_at=?
	WHERE id=?`,
		[]any{
			r.Name, r.Endpoint, r.PasswordEncrypted,
			r.RcloneConfigEncrypted, r.WebdavURL, r.WebdavUser, r.WebdavPasswordEncrypted,
			r.Options, r.PruneEnabled, r.PruneCronExpr, r.PruneArgs,
			r.CheckEnabled, r.CheckCronExpr, r.CheckArgs, now, r.ID,
		}
}

func (s *RepoStore) hasColumn(table, column string) bool {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false
		}
		if strings.EqualFold(name, column) {
			return true
		}
	}
	return false
}
