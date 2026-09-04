import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Mark } from '../api/types'

export interface Board {
  marks: Mark[]
  error: string | null
  loading: boolean
  pull: () => void
  absorb: (mark: Mark) => void
}

// Holds the mark log and a cursor. Every fetch asks only for what came after the
// cursor, so the first load, a live update and a reconnect are the same request.
export function useBoard(): Board {
  const [marks, setMarks] = useState<Mark[]>([])
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const cursor = useRef(0)
  const busy = useRef(false)
  const [tick, setTick] = useState(0)

  const pull = useCallback(() => setTick((n) => n + 1), [])

  // A mark this client just created is already known, so it is folded in directly
  // rather than waiting for the round trip its own request will trigger.
  const absorb = useCallback((mark: Mark) => {
    if (mark.seq <= cursor.current) return
    cursor.current = mark.seq
    setMarks((existing) => [...existing, mark])
  }, [])

  useEffect(() => {
    let active = true

    const run = async () => {
      if (busy.current) return
      busy.current = true

      try {
        // A backlog comes back a page at a time, so keep asking until it runs dry.
        for (let page = 0; page < 20; page++) {
          const next = await api.boardSince(cursor.current)
          if (!active) return

          if (next.marks.length > 0) {
            cursor.current = next.cursor
            setMarks((existing) => [...existing, ...next.marks])
          }

          if (!next.more) break
        }

        if (active) setError(null)
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'could not read the board')
      } finally {
        busy.current = false
        if (active) setLoading(false)
      }
    }

    run()
    const id = setInterval(run, 6000)

    return () => {
      active = false
      clearInterval(id)
    }
  }, [tick])

  return { marks, error, loading, pull, absorb }
}
