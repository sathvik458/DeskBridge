import type { Device } from './api/types'

export function sinceLabel(iso: string | null, now: number): string {
  if (iso === null) return 'never seen'

  const seconds = Math.max(0, Math.round((now - Date.parse(iso)) / 1000))

  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`

  return `${Math.floor(seconds / 86400)}d ago`
}

export function clockLabel(iso: string | null): string {
  if (iso === null) return 'unknown'
  const d = new Date(iso)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function deviceTag(device: Device, now: number): { tone: 'ok' | 'quiet'; text: string } {
  if (device.status === 'online') {
    return { tone: 'ok', text: `Online · last seen ${sinceLabel(device.last_seen_at, now)}` }
  }
  if (device.last_seen_at === null) {
    return { tone: 'quiet', text: 'Never seen' }
  }
  return { tone: 'quiet', text: `Quiet since ${clockLabel(device.last_seen_at)}` }
}
