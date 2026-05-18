export type AsyncJobStatus =
  | 'idle'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'cancelled'
  | 'partial'
  | 'stale'
  | 'unknown'
  | (string & {})

export type RepoSyncDomain = 'core' | 'files' | 'stats' | 'keys' | 'all'
export type ExecutionStatus = 'running' | 'success' | 'failed' | 'cancelled'
export type ExecutionTrigger = 'manual' | 'scheduled' | 'system_query'

export interface Repository {
  id: number
  name: string
  type: 'local' | 'rclone' | 'webdav'
  endpoint: string
  options: string
  has_rclone_config?: boolean
  prune_enabled: boolean
  prune_cron_expr: string
  prune_args: string
  check_enabled: boolean
  check_cron_expr: string
  check_args: string
  sync_status?: AsyncJobStatus | null
  last_synced_at?: string | null
  snapshot_count?: number | null
  file_index_status?: AsyncJobStatus | null
  last_check_status?: AsyncJobStatus | null
  last_check_at?: string | null
  created_at: string
  updated_at: string
}

export interface CreateRepoRequest {
  name: string
  type: string
  endpoint: string
  password: string
  rclone_config?: string
  webdav_url?: string
  webdav_user?: string
  webdav_password?: string
  options?: string
  init_on_create?: boolean
  auto_unlock?: boolean
  prune_enabled?: boolean
  prune_cron_expr?: string
  prune_args?: string
  check_enabled?: boolean
  check_cron_expr?: string
  check_args?: string
}

export interface RepositoryAccessCheckResponse {
  exists: boolean
  accessible: boolean
  locked?: boolean
}

export interface BackupTask {
  id: number
  repo_id: number
  repo_name?: string
  name: string
  source_paths: string
  excludes: string
  tags: string
  cron_expr: string
  cron_enabled: boolean
  forget_policy: string
  pre_hooks: string
  post_hooks: string
  extra_flags: string
  created_at: string
  updated_at: string
  last_run_at?: string
  next_run_at?: string
}

export interface ExecutionLog {
  id: number
  repo_id?: number
  repo_name?: string
  task_id?: number
  command: string
  stdout: string
  stderr: string
  combined_output: string
  exit_code: number
  status: ExecutionStatus
  trigger: ExecutionTrigger
  started_at: string
  finished_at?: string
  duration_ms: number
}

export interface OperationResponse {
  log_id?: number
  init_log_id?: number
  status?: ExecutionStatus | 'queued'
  exit_code?: number
  stdout?: string
  stderr?: string
  id?: number
  import_status?: AsyncJobStatus | null
  import_log_id?: number | null
  unlock_log_id?: number | null
}

export interface ExecutionStreamMessage {
  execution_id: number
  type: 'output' | 'complete' | 'error'
  time?: string
  stream?: 'stdout' | 'stderr'
  text?: string
  exit_code?: number
}

export interface Snapshot {
  id: string
  short_id: string
  time: string
  hostname: string
  username?: string
  uid?: number
  gid?: number
  tags: string[]
  paths: string[]
  tree: string
  program_version?: string
  summary?: Record<string, unknown>
  backup_start?: string
  backup_end?: string
  files_new?: number
  files_changed?: number
  files_unmodified?: number
  dirs_new?: number
  dirs_changed?: number
  dirs_unmodified?: number
  data_blobs?: number
  tree_blobs?: number
  data_added?: number
  data_added_packed?: number
  total_files_processed?: number
  total_bytes_processed?: number
}

export interface SnapshotPage {
  items: Snapshot[]
  total: number
  page: number
  page_size: number
  indexing: boolean
  sync_status?: AsyncJobStatus | null
  partial?: boolean
  indexed_snapshot_count?: number | null
  stale?: boolean
  last_indexed_at?: string | null
  error?: string | null
}

export interface RepositorySyncDomainState {
  status?: AsyncJobStatus | null
  last_synced_at?: string | null
  updated_at?: string | null
  log_id?: number | null
  error?: string | null
}

export type RepositorySyncState = Partial<
  Record<RepoSyncDomain, AsyncJobStatus | RepositorySyncDomainState | null | undefined>
>

export interface SnapshotFileItem {
  name: string
  path?: string
  type: 'file' | 'dir'
  size?: number
}

export interface SnapshotFilesPage {
	items: SnapshotFileItem[]
	indexing?: boolean
	stale?: boolean
	error?: string | null
	indexed_at?: string | null
	total?: number | null
}

export interface RepoStatsView {
	data?: unknown
	indexing?: boolean
	stale?: boolean
	error?: string | null
	indexed_at?: string | null
}
