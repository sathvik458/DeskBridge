import type { FocusKind } from './api/types'

export const roundLength = 4

export function kindLabel(kind: FocusKind): string {
  if (kind === 'short_break') return 'Short break'
  if (kind === 'long_break') return 'Long break'
  return 'Focus'
}

// Server sends the seconds left and the instant it measured them. The browser only
// subtracts the time that has passed locally since, so a wrong clock cannot skew it.
export function secondsLeft(leftSeconds: number, serverTime: string, now: number): number {
  const drift = (now - Date.parse(serverTime)) / 1000
  return Math.max(0, Math.round(leftSeconds - Math.max(0, drift)))
}

export function countdown(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const m = Math.floor(safe / 60)
  const s = safe % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

// 0 at the start, 1 when the stretch is over. Used to sweep the ring round.
export function swept(leftSeconds: number, plannedSeconds: number): number {
  if (plannedSeconds <= 0) return 1
  return Math.min(1, Math.max(0, 1 - leftSeconds / plannedSeconds))
}

export function dots(intoRound: number): boolean[] {
  return Array.from({ length: roundLength }, (_, i) => i < intoRound)
}
