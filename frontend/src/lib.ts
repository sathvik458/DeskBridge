import type { Device, Session } from './api/types'

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

export function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds))
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  const pad = (n: number) => String(n).padStart(2, '0')

  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`
}

// The server sends elapsed_seconds alongside the instant it measured them. The browser
// only adds the time that has passed locally since, and only while the clock is running,
// so a wrong local clock cannot corrupt the count.
export function liveElapsed(session: Session, now: number): number {
  if (session.status !== 'active') return session.elapsed_seconds

  const drift = (now - Date.parse(session.server_time)) / 1000

  return session.elapsed_seconds + Math.max(0, drift)
}

export function todayISO(): string {
  const d = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
