import { useEffect, useRef, useState } from 'react'

export type FeedState = 'connecting' | 'live' | 'dropped'

type Handlers = Record<string, () => void>

const base = import.meta.env.VITE_API_BASE ?? ''

// EventSource reconnects on its own, so there is no retry loop here. All this hook
// does is surface the connection state and fan events out to the pollers, which
// still hold the authoritative data.
export function useLiveFeed(handlers: Handlers): FeedState {
  const [state, setState] = useState<FeedState>('connecting')

  const latest = useRef(handlers)
  latest.current = handlers

  useEffect(() => {
    const source = new EventSource(base + '/api/live')

    source.onopen = () => setState('live')
    source.onerror = () => setState('dropped')

    const attached = Object.keys(latest.current)

    const listen = (kind: string) => {
      const react = () => {
        setState('live')
        latest.current[kind]?.()
      }
      source.addEventListener(kind, react)
      return react
    }

    const wired = attached.map((kind) => [kind, listen(kind)] as const)

    return () => {
      wired.forEach(([kind, react]) => source.removeEventListener(kind, react))
      source.close()
    }
  }, [])

  return state
}
