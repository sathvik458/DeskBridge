import { useCallback, useEffect, useRef, useState } from 'react'

export interface Poll<T> {
  data: T | null
  error: string | null
  loading: boolean
  stale: boolean
  refresh: () => void
}

// Keeps the last good value on screen when a request fails, and flags it as stale,
// because a dropped VPN should not blank the dashboard.
export function usePoll<T>(fetcher: () => Promise<T>, intervalMs: number): Poll<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher

  const [tick, setTick] = useState(0)
  const refresh = useCallback(() => setTick((n) => n + 1), [])

  useEffect(() => {
    let active = true

    const run = async () => {
      try {
        const next = await fetcherRef.current()
        if (!active) return
        setData(next)
        setError(null)
      } catch (err) {
        if (!active) return
        setError(err instanceof Error ? err.message : 'request failed')
      } finally {
        if (active) setLoading(false)
      }
    }

    run()
    const id = setInterval(run, intervalMs)

    return () => {
      active = false
      clearInterval(id)
    }
  }, [intervalMs, tick])

  return { data, error, loading, stale: error !== null && data !== null, refresh }
}
