import type { Component } from 'vue'
import { h } from 'vue'
import { NIcon } from 'naive-ui'
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Database,
  HelpCircle,
  RotateCw,
  type LucideIcon,
  TerminalSquare,
  XCircle
} from '@lucide/vue'
import type { AsyncJobStatus, ExecutionLog } from '../types'

export function executionStatusLabel(status?: ExecutionLog['status'] | string | null): string {
  switch (status) {
    case 'success':
      return '成功'
    case 'failed':
      return '失败'
    case 'running':
      return '运行中'
    case 'cancelled':
      return '已取消'
    default:
      return '未知'
  }
}

export function executionStatusType(status?: ExecutionLog['status'] | string | null): 'success' | 'error' | 'warning' | 'info' | 'default' {
  switch (status) {
    case 'success':
      return 'success'
    case 'failed':
      return 'error'
    case 'running':
      return 'info'
    case 'cancelled':
      return 'warning'
    default:
      return 'default'
  }
}

export function executionStatusIcon(status?: ExecutionLog['status'] | string | null): LucideIcon {
  switch (status) {
    case 'success':
      return CheckCircle2
    case 'failed':
      return XCircle
    case 'running':
      return RotateCw
    case 'cancelled':
      return AlertTriangle
    default:
      return Clock3
  }
}

export function executionTriggerLabel(trigger?: ExecutionLog['trigger'] | string | null): string {
  switch (trigger) {
    case 'manual':
      return '手动'
    case 'scheduled':
      return '定时'
    case 'system_query':
      return '系统'
    default:
      return '未知'
  }
}

export function renderNaiveIcon(icon: Component, size = 16) {
  return () => h(NIcon, { size }, { default: () => h(icon) })
}

export function streamPrefix(stream?: 'stdout' | 'stderr' | string | null): string {
  if (stream === 'stderr') return '[stderr]'
  if (stream === 'stdout') return '[stdout]'
  return '[log]'
}

export function jobStatusLabel(status?: AsyncJobStatus | null): string {
  switch (status) {
    case 'success':
      return '已完成'
    case 'failed':
      return '失败'
    case 'running':
      return '运行中'
    case 'queued':
      return '排队中'
    case 'partial':
      return '部分可用'
    case 'stale':
      return '结果过期'
    case 'idle':
      return '未启动'
    case 'cancelled':
      return '已取消'
    default:
      return status ? String(status) : '未知'
  }
}

export function jobStatusType(status?: AsyncJobStatus | null): 'success' | 'error' | 'warning' | 'info' | 'default' {
  switch (status) {
    case 'success':
      return 'success'
    case 'failed':
      return 'error'
    case 'running':
    case 'queued':
      return 'info'
    case 'partial':
    case 'stale':
    case 'cancelled':
      return 'warning'
    default:
      return 'default'
  }
}

export function jobStatusIcon(status?: AsyncJobStatus | null): LucideIcon {
  switch (status) {
    case 'success':
      return CheckCircle2
    case 'failed':
      return XCircle
    case 'running':
    case 'queued':
      return RotateCw
    case 'partial':
    case 'stale':
    case 'cancelled':
      return AlertTriangle
    case 'idle':
      return Database
    default:
      return HelpCircle
  }
}

export const terminalIcon = TerminalSquare
