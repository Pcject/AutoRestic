export interface ParsedResticBackupCommand {
  sourcePaths: string[]
  excludes: string[]
  tags: string[]
  extraFlags: Record<string, string | number | boolean>
}

const valueFlags = new Set([
  '--host',
  '--compression',
  '--pack-size',
  '--parent',
  '--limit-upload',
  '--limit-download',
  '--read-concurrency',
  '--exclude-file',
  '--files-from',
  '--exclude-larger-than',
  '--exclude-if-present',
  '--iexclude',
  '--iexclude-file',
  '--retry-lock',
  '--group-by',
  '--option'
])

const booleanFlags = new Set([
  '--force',
  '--ignore-ctime',
  '--ignore-inode',
  '--one-file-system',
  '--no-scan',
  '--with-atime',
  '--exclude-caches',
  '--quiet',
  '--json',
  '--no-lock',
  '--dry-run',
  '--skip-if-unchanged',
  '--no-cache'
])

const numberFlags = new Set(['--pack-size', '--limit-upload', '--limit-download', '--read-concurrency'])

export function parseResticBackupCommand(raw: string): ParsedResticBackupCommand {
  const tokens = tokenizeShellLike(raw)
  if (!tokens.length) {
    throw new Error('请粘贴 restic backup 命令')
  }

  let index = 0
  while (tokens[index]?.includes('=') && !tokens[index].startsWith('-')) {
    index++
  }
  const startsWithRestic = tokens[index] === 'restic'
  if (startsWithRestic) {
    index++
    while (tokens[index] && tokens[index] !== 'backup') {
      if (!tokens[index].startsWith('-')) break
      index++
      if (tokens[index] && !tokens[index].startsWith('-')) index++
    }
  }
  if (tokens[index] === 'backup') {
    index++
  } else if (startsWithRestic || looksLikeAnotherResticCommand(tokens[index])) {
    throw new Error('只支持解析 restic backup 命令')
  }

  const parsed: ParsedResticBackupCommand = {
    sourcePaths: [],
    excludes: [],
    tags: [],
    extraFlags: {}
  }
  let optionsEnded = false

  for (; index < tokens.length; index++) {
    const token = tokens[index]
    if (!token) continue
    if (token === '--') {
      optionsEnded = true
      continue
    }
    if (!optionsEnded && token === '-v') {
      parsed.extraFlags['--verbose'] = true
      continue
    }
    if (!optionsEnded && token === '-vv') {
      parsed.extraFlags['--verbose'] = 2
      continue
    }
    if (!optionsEnded && token === '-n') {
      parsed.extraFlags['--dry-run'] = true
      continue
    }
    if (!optionsEnded && token.startsWith('--')) {
      const [flag, inlineValue] = splitLongFlag(token)
      if (flag === '--exclude' || flag === '--tag') {
        const value = inlineValue ?? nextValue(tokens, ++index, flag)
        if (flag === '--exclude') parsed.excludes.push(value)
        else parsed.tags.push(value)
        continue
      }
      if (booleanFlags.has(flag)) {
        if (inlineValue !== undefined) {
          parsed.extraFlags[flag] = parseBooleanValue(inlineValue)
        } else {
          parsed.extraFlags[flag] = true
        }
        continue
      }
      if (valueFlags.has(flag)) {
        const value = inlineValue ?? nextValue(tokens, ++index, flag)
        parsed.extraFlags[flag] = coerceFlagValue(flag, value)
        continue
      }
      throw new Error(`不支持的 backup 参数: ${flag}`)
    }
    if (!optionsEnded && token.startsWith('-')) {
      throw new Error(`不支持的短参数: ${token}`)
    }
    parsed.sourcePaths.push(token)
  }

  if (!parsed.sourcePaths.length) {
    throw new Error('命令中没有识别到源路径')
  }
  return parsed
}

function looksLikeAnotherResticCommand(token: string | undefined) {
  return !!token && new Set([
    'snapshots',
    'restore',
    'check',
    'prune',
    'forget',
    'unlock',
    'init',
    'stats',
    'ls',
    'find',
    'diff',
    'cat',
    'key',
    'copy',
    'dump',
    'mount',
    'repair',
    'rewrite',
    'self-update',
    'version'
  ]).has(token)
}

function splitLongFlag(token: string): [string, string | undefined] {
  const index = token.indexOf('=')
  if (index < 0) return [token, undefined]
  return [token.slice(0, index), token.slice(index + 1)]
}

function nextValue(tokens: string[], index: number, flag: string) {
  const value = tokens[index]
  if (value === undefined || value === '') {
    throw new Error(`${flag} 缺少参数值`)
  }
  return value
}

function coerceFlagValue(flag: string, value: string) {
  if (!numberFlags.has(flag)) return value
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) {
    throw new Error(`${flag} 需要数字参数`)
  }
  return numeric
}

function parseBooleanValue(value: string) {
  if (['1', 'true', 'yes', 'on'].includes(value.toLowerCase())) return true
  if (['0', 'false', 'no', 'off'].includes(value.toLowerCase())) return false
  throw new Error(`布尔参数值无效: ${value}`)
}

function tokenizeShellLike(raw: string) {
  const input = raw.replace(/\\\r?\n/g, ' ')
  const tokens: string[] = []
  let current = ''
  let quote: '"' | "'" | null = null
  let escaped = false

  for (const char of input) {
    if (escaped) {
      current += char
      escaped = false
      continue
    }
    if (char === '\\' && quote !== "'") {
      escaped = true
      continue
    }
    if (quote) {
      if (char === quote) {
        quote = null
      } else {
        current += char
      }
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
      continue
    }
    if (/\s/.test(char)) {
      if (current) {
        tokens.push(current)
        current = ''
      }
      continue
    }
    current += char
  }

  if (escaped) current += '\\'
  if (quote) {
    throw new Error('命令中存在未闭合的引号')
  }
  if (current) tokens.push(current)
  return tokens
}
