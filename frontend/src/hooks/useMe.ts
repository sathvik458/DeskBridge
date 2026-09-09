import { useCallback, useEffect, useState } from 'react'
import { api, RequestFailed } from '../api/client'
import type { Me } from '../api/types'

type State = 'asking' | 'in' | 'out'

// One question on load: does this browser already hold a good cookie? A 401 is a
// normal answer here, not an error worth showing anybody.
export function useMe() {
  const [me, setMe] = useState<Me | null>(null)
  const [state, setState] = useState<State>('asking')

  useEffect(() => {
    let live = true

    api
      .me()
      .then((who) => {
        if (!live) return
        setMe(who)
        setState('in')
      })
      .catch((err) => {
        if (!live) return
        if (!(err instanceof RequestFailed) || err.status !== 401) {
          console.warn('could not check the session', err)
        }
        setState('out')
      })

    return () => {
      live = false
    }
  }, [])

  const arrived = useCallback((who: Me) => {
    setMe(who)
    setState('in')
  }, [])

  const left = useCallback(async () => {
    try {
      await api.signOut()
    } finally {
      setMe(null)
      setState('out')
    }
  }, [])

  return { me, state, arrived, left }
}
