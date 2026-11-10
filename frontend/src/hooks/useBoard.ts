import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { Mark } from '../api/types'
import { mergeMarks } from '../board'

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
  const again = useRef(false)
  const alive = useRef(true)

  // A pull that lands while a fetch is already going is remembered, not dropped.
  // Otherwise a live event arriving mid-fetch left the board a whole poll behind.
  const pull = useCallback(async () => {
    if (busy.current) {
      again.current = true
      return
    }
    busy.current = true

    try {
      do {
        again.current = false

        // A backlog comes back a page at a time, so keep asking until it runs dry.
        for (let page = 0; page < 20; page++) {
          const next = await api.boardSince(cursor.current)
          if (!alive.current) return

          if (next.marks.length > 0) {
            cursor.current = Math.max(cursor.current, next.cursor)
            setMarks((known) => mergeMarks(known, next.marks))
          }

          if (!next.more) break
        }
      } while (again.current && alive.current)

      if (alive.current) setError(null)
    } catch (err) {
      if (alive.current) setError(err instanceof Error ? err.message : 'could not read the board')
    } finally {
      busy.current = false
      if (alive.current) setLoading(false)
    }
  }, [])

  // My own mark goes on screen straight away, but it no longer moves the cursor.
  // Jumping the cursor to it skipped any mark the other person made just before.
  const absorb = useCallback((mark: Mark) => {
    setMarks((known) => mergeMarks(known, [mark]))
  }, [])

  useEffect(() => {
    alive.current = true
    pull()
    const id = setInterval(pull, 6000)

    return () => {
      alive.current = false
      clearInterval(id)
    }
  }, [pull])

  return { marks, error, loading, pull, absorb }
}
