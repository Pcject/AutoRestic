import { h } from 'vue'

export function confirmCommandContent(description: string, command: string) {
  return () =>
    h('div', { style: 'display: grid; gap: 8px;' }, [
      h('p', description),
      h('span', { style: 'color: var(--text-secondary); font-size: 12px;' }, '命令预览'),
      h('pre', {
        style: 'max-height: 240px; overflow: auto; margin: 0; padding: 10px 12px; border: 1px solid var(--border-color); border-radius: 6px; background: #11181d; color: #d6f5df; font-family: var(--font-mono); white-space: pre-wrap;'
      }, command)
    ])
}
