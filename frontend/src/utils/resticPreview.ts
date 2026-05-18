import type { BackupTask, RepoSyncDomain, Repository } from '../types'

export function shellQuote(value: string) {
  if (/^[A-Za-z0-9_./:=+,%@-]+$/.test(value)) return value
  return "'" + value.replace(/'/g, "'\\''") + "'"
}

export function resticRepositoryPreview(repo?: Pick<Repository, 'type' | 'endpoint'> | null) {
  if (!repo) return ''
  if (repo.type === 'rclone') return `rclone:${repo.endpoint}`
  if (repo.type === 'webdav') return `webdav:${repo.endpoint}`
  return repo.endpoint
}

export function resticCommandPreview(repo: Pick<Repository, 'type' | 'endpoint'> | null | undefined, args: string[]) {
  const repository = resticRepositoryPreview(repo)
  const env = repository ? `RESTIC_REPOSITORY=${shellQuote(repository)} ` : ''
  return `${env}restic ${args.map(shellQuote).join(' ')}`
}

export function jsonArray(raw: string, fallback: string[] = []) {
  try {
    const value = JSON.parse(raw || '[]')
    return Array.isArray(value) ? value.map(String).filter(Boolean) : fallback
  } catch {
    return fallback
  }
}

function jsonObject(raw: string) {
  try {
    const value = JSON.parse(raw || '{}')
    return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
  } catch {
    return {}
  }
}

function buildExtraFlagArgs(raw: string) {
  const flags = jsonObject(raw)
  const args: string[] = []
  Object.keys(flags).sort().forEach(key => {
    const value = flags[key]
    if (value === true) {
      args.push(key)
      return
    }
    if (key === '--verbose' && value === 2) {
      args.push('-vv')
      return
    }
    if (value !== false && value !== null && value !== undefined && value !== '') {
      args.push(key, String(value))
    }
  })
  return args
}

function buildForgetPolicyArgs(raw: string) {
  const policy = jsonObject(raw)
  const args = ['forget']
  const entries: Array<[string, string[]]> = [
    ['--keep-last', ['keep_last', 'keep-last']],
    ['--keep-daily', ['keep_daily', 'keep-daily']],
    ['--keep-weekly', ['keep_weekly', 'keep-weekly']],
    ['--keep-monthly', ['keep_monthly', 'keep-monthly']],
    ['--keep-yearly', ['keep_yearly', 'keep-yearly']]
  ]
  entries.forEach(([flag, keys]) => {
    const value = keys.map(key => policy[key]).find(item => item !== undefined && item !== null && item !== '')
    if (value !== undefined && value !== null && value !== '') {
      args.push(flag, String(value))
    }
  })
  return args.length > 1 ? args : []
}

export function taskRunCommandPreview(task: BackupTask, repo?: Repository) {
  const args = ['backup']
  args.push(...buildExtraFlagArgs(task.extra_flags))
  if (!args.includes('--json')) args.push('--json')
  jsonArray(task.excludes).forEach(item => args.push('--exclude', item))
  jsonArray(task.tags).forEach(item => args.push('--tag', item))
  args.push(...jsonArray(task.source_paths))

  const lines = [resticCommandPreview(repo, args)]
  const forgetArgs = buildForgetPolicyArgs(task.forget_policy)
  if (forgetArgs.length) {
    lines.push('# 备份成功后会按保留策略继续执行')
    lines.push(resticCommandPreview(repo, forgetArgs))
  }
  return lines.join('\n')
}

export function repoSyncCommandPreview(repo: Pick<Repository, 'type' | 'endpoint'>, domain: RepoSyncDomain) {
  const commands: Record<RepoSyncDomain, string[]> = {
    core: [
      resticCommandPreview(repo, ['cat', 'config']),
      resticCommandPreview(repo, ['snapshots', '--json'])
    ],
    files: [
      resticCommandPreview(repo, ['ls', '--json', '[snapshot-id]', '[path]'])
    ],
    stats: [
      resticCommandPreview(repo, ['stats', '--json'])
    ],
    keys: [
      resticCommandPreview(repo, ['key', 'list', '--json'])
    ],
    all: []
  }
  commands.all = [...commands.core, ...commands.stats, ...commands.keys, ...commands.files]
  return commands[domain].join('\n')
}
