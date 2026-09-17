import { describe, expect, it } from 'vitest'
import {
  clockLabel,
  formatDuration,
  hoursLabel,
  liveElapsed,
  shortDay,
  sinceLabel,
  sizeLabel,
} from './lib'
import type { Session } from './api/types'

describe('formatDuration', () => {
  it('drops the hour until there is one', () => {
    expect(formatDuration(59)).toBe('00:59')
    expect(formatDuration(600)).toBe('10:00')
    expect(formatDuration(3600)).toBe('1:00:00')
    expect(formatDuration(3725)).toBe('1:02:05')
  })

  it('clamps anything negative', () => {
    expect(formatDuration(-5)).toBe('00:00')
  })
})

describe('sinceLabel', () => {
  const now = Date.parse('2026-09-17T12:00:00Z')

  it('says never when there is nothing to go on', () => {
    expect(sinceLabel(null, now)).toBe('never seen')
  })

  it('steps up through the units', () => {
    expect(sinceLabel('2026-09-17T11:59:58Z', now)).toBe('just now')
    expect(sinceLabel('2026-09-17T11:59:20Z', now)).toBe('40s ago')
    expect(sinceLabel('2026-09-17T11:30:00Z', now)).toBe('30m ago')
    expect(sinceLabel('2026-09-17T09:00:00Z', now)).toBe('3h ago')
    expect(sinceLabel('2026-09-15T12:00:00Z', now)).toBe('2d ago')
  })

  // A device whose clock runs ahead would otherwise show a negative age.
  it('does not go backwards for a future timestamp', () => {
    expect(sinceLabel('2026-09-17T12:05:00Z', now)).toBe('just now')
  })
})

describe('liveElapsed', () => {
  const base: Session = {
    id: 's1',
    subject: 'Physics',
    topic: null,
    status: 'active',
    started_at: '2026-09-17T09:00:00Z',
    ended_at: null,
    elapsed_seconds: 600,
    server_time: '2026-09-17T09:10:00Z',
  }

  const at = Date.parse(base.server_time)

  it('adds the time that has passed since the server measured it', () => {
    expect(liveElapsed(base, at + 30_000)).toBe(630)
  })

  it('holds still once the session is not active', () => {
    expect(liveElapsed({ ...base, status: 'paused' }, at + 30_000)).toBe(600)
    expect(liveElapsed({ ...base, status: 'completed' }, at + 90_000)).toBe(600)
  })

  // The server's number is the floor; a slow local clock must not walk it back.
  it('never subtracts time when the local clock is behind', () => {
    expect(liveElapsed(base, at - 60_000)).toBe(600)
  })
})

describe('sizeLabel', () => {
  it('picks a unit', () => {
    expect(sizeLabel(512)).toBe('512 B')
    expect(sizeLabel(2048)).toBe('2 KB')
    expect(sizeLabel(5 * 1024 * 1024)).toBe('5.0 MB')
  })
})

describe('hoursLabel', () => {
  it('reads as minutes until there is an hour', () => {
    expect(hoursLabel(0)).toBe('0m')
    expect(hoursLabel(1800)).toBe('30m')
    expect(hoursLabel(3600)).toBe('1h')
    expect(hoursLabel(5400)).toBe('1h 30m')
  })
})

describe('clockLabel and shortDay', () => {
  it('says unknown with nothing to show', () => {
    expect(clockLabel(null)).toBe('unknown')
  })

  it('names the weekday', () => {
    expect(shortDay('2026-09-17')).toBe('Thu')
  })
})
