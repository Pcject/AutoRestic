export type LogViewMode = 'tail' | 'head'

export interface LogDisplaySlice {
  text: string
  totalLines: number
  shownLines: number
  hiddenLines: number
  truncated: boolean
}

export const DEFAULT_LOG_LINE_LIMIT = 400
export const MAX_STREAMED_LOG_LINES = 1200
export const LOG_LINE_LIMIT_OPTIONS = [200, 400, 800, 1200, 2000]

export function splitLogText(text?: string | null): string[] {
  if (!text) return []
  return text.replace(/\r\n/g, '\n').split('\n')
}

export function buildLogSlice(
  text: string | null | undefined,
  mode: LogViewMode,
  limit: number
): LogDisplaySlice {
  const lines = splitLogText(text)
  const normalizedLimit = Math.max(1, Math.floor(limit) || DEFAULT_LOG_LINE_LIMIT)
  const shownLines =
    mode === 'head'
      ? lines.slice(0, normalizedLimit)
      : lines.slice(Math.max(lines.length - normalizedLimit, 0))

  return {
    text: shownLines.join('\n'),
    totalLines: lines.length,
    shownLines: shownLines.length,
    hiddenLines: Math.max(lines.length - shownLines.length, 0),
    truncated: lines.length > shownLines.length
  }
}

export function pushStreamedLines(
  currentLines: string[],
  nextLines: string[],
  maxLines = MAX_STREAMED_LOG_LINES
): { lines: string[]; dropped: number } {
  const merged = currentLines.concat(nextLines)
  if (merged.length <= maxLines) {
    return { lines: merged, dropped: 0 }
  }

  const dropped = merged.length - maxLines
  return {
    lines: merged.slice(dropped),
    dropped
  }
}
