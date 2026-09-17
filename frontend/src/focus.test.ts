import { describe, expect, it } from 'vitest'
import { countdown, dots, kindLabel, roundLength, secondsLeft, swept } from './focus'

describe('kindLabel', () => {
  it('names each kind', () => {
    expect(kindLabel('focus')).toBe('Focus')
    expect(kindLabel('short_break')).toBe('Short break')
    expect(kindLabel('long_break')).toBe('Long break')
  })
})

describe('secondsLeft', () => {
  const measured = '2026-09-17T09:00:00Z'
  const at = Date.parse(measured)

  it('returns what the server said when no time has passed', () => {
    expect(secondsLeft(1500, measured, at)).toBe(1500)
  })

  it('subtracts the time that has passed locally', () => {
    expect(secondsLeft(1500, measured, at + 60_000)).toBe(1440)
  })

  it('never goes below zero', () => {
    expect(secondsLeft(30, measured, at + 600_000)).toBe(0)
  })

  // A machine whose clock is behind the server would otherwise show more time left
  // than there really is, which is the wrong direction to be wrong in.
  it('ignores a local clock that is behind the server', () => {
    expect(secondsLeft(1500, measured, at - 300_000)).toBe(1500)
  })
})

describe('countdown', () => {
  it('pads both halves', () => {
    expect(countdown(1500)).toBe('25:00')
    expect(countdown(65)).toBe('01:05')
    expect(countdown(9)).toBe('00:09')
    expect(countdown(0)).toBe('00:00')
  })

  it('clamps anything negative', () => {
    expect(countdown(-30)).toBe('00:00')
  })

  it('does not roll over at an hour, because no stretch is that long', () => {
    expect(countdown(3660)).toBe('61:00')
  })
})

describe('swept', () => {
  it('runs from nothing to everything', () => {
    expect(swept(1500, 1500)).toBe(0)
    expect(swept(750, 1500)).toBe(0.5)
    expect(swept(0, 1500)).toBe(1)
  })

  it('stays inside 0 and 1 whatever it is given', () => {
    expect(swept(2000, 1500)).toBe(0)
    expect(swept(-100, 1500)).toBe(1)
  })

  // Dividing by zero would give NaN and the ring would vanish.
  it('treats a zero plan as finished rather than dividing by it', () => {
    expect(swept(0, 0)).toBe(1)
  })
})

describe('dots', () => {
  it('fills one per finished stretch', () => {
    expect(dots(0)).toEqual([false, false, false, false])
    expect(dots(2)).toEqual([true, true, false, false])
    expect(dots(roundLength)).toEqual([true, true, true, true])
  })
})
