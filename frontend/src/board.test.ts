import { describe, expect, it } from 'vitest'
import { strokesFrom, strokeUnder } from './board'
import type { Mark } from './api/types'

const draw = (seq: number, id: string, path: number[]): Mark => ({
  seq,
  id,
  kind: 'draw',
  by: 'student',
  ink: '#1E1E1E',
  thickness: 3,
  path,
  created_at: '2026-09-17T09:00:00Z',
})

const erase = (seq: number, id: string, target: string): Mark => ({
  seq,
  id,
  kind: 'erase',
  by: 'student',
  target_id: target,
  created_at: '2026-09-17T09:00:00Z',
})

const clear = (seq: number, id: string): Mark => ({
  seq,
  id,
  kind: 'clear',
  by: 'student',
  created_at: '2026-09-17T09:00:00Z',
})

const ids = (marks: Mark[]) => strokesFrom(marks).map((s) => s.id)

describe('strokesFrom', () => {
  it('keeps plain draws in order', () => {
    expect(ids([draw(1, 'a', [0, 0]), draw(2, 'b', [0, 1])])).toEqual(['a', 'b'])
  })

  it('drops a stroke an erase points at', () => {
    expect(ids([draw(1, 'a', [0, 0]), draw(2, 'b', [0, 1]), erase(3, 'e', 'a')])).toEqual(['b'])
  })

  it('hides everything before a clear', () => {
    expect(ids([draw(1, 'a', [0, 0]), draw(2, 'b', [0, 1]), clear(3, 'c')])).toEqual([])
  })

  it('keeps what was drawn after a clear', () => {
    expect(ids([draw(1, 'a', [0, 0]), clear(2, 'c'), draw(3, 'b', [0, 1])])).toEqual(['b'])
  })

  it('applies an erase that lands after a clear', () => {
    expect(ids([clear(1, 'c'), draw(2, 'a', [0, 0]), draw(3, 'b', [0, 1]), erase(4, 'e', 'a')])).toEqual(['b'])
  })

  it('only honours the newest clear', () => {
    const log = [draw(1, 'a', [0, 0]), clear(2, 'c'), draw(3, 'b', [0, 1]), clear(4, 'd'), draw(5, 'z', [1, 1])]
    expect(ids(log)).toEqual(['z'])
  })

  it('copes with an empty log', () => {
    expect(ids([])).toEqual([])
  })
})

describe('strokeUnder', () => {
  const strokes = strokesFrom([draw(1, 'line', [0.1, 0.5, 0.9, 0.5]), draw(2, 'dot', [0.5, 0.1])])

  it('finds a stroke under the click', () => {
    expect(strokeUnder(strokes, 0.5, 0.5, 0.02)).toBe('line')
  })

  it('misses when the click is off the line', () => {
    expect(strokeUnder(strokes, 0.5, 0.53, 0.02)).toBeNull()
  })

  it('does not reach past the end of a segment', () => {
    expect(strokeUnder(strokes, 0.02, 0.5, 0.02)).toBeNull()
  })

  it('can hit a single point stroke', () => {
    expect(strokeUnder(strokes, 0.5, 0.105, 0.02)).toBe('dot')
  })

  // Clicking where two overlap should pick the one on top, which is the later one.
  it('prefers the stroke drawn last', () => {
    const stacked = strokesFrom([draw(1, 'under', [0.4, 0.4, 0.6, 0.6]), draw(2, 'over', [0.4, 0.4, 0.6, 0.6])])
    expect(strokeUnder(stacked, 0.5, 0.5, 0.02)).toBe('over')
  })
})
