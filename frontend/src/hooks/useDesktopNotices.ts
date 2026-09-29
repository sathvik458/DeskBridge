import { useCallback, useEffect, useRef, useState } from 'react'
import type { Notice, NoticeList } from '../api/types'

export type AlertState = 'unsupported' | 'off' | 'on' | 'blocked'

// A bump keeps the notice id and moves made_at, so this changes when the same
// thing happens again and stays put when nothing has.
const keyOf = (n: Notice) => `${n.id}:${n.made_at}`

function currentState(): AlertState {
  if (typeof window === 'undefined' || !('Notification' in window)) return 'unsupported'

  switch (Notification.permission) {
    case 'granted':
      return 'on'
    case 'denied':
      return 'blocked'
    default:
      return 'off'
  }
}

// Desktop popups are driven off the notice list rather than the live feed, so the
// wording is the server's. Composing a second version of every sentence in the
// browser would be two places to keep in step.
export function useDesktopNotices(list: NoticeList | null) {
  const [state, setState] = useState<AlertState>(currentState)

  const shown = useRef(new Set<string>())
  const primed = useRef(false)

  const ask = useCallback(async () => {
    if (!('Notification' in window)) return
    await Notification.requestPermission()
    setState(currentState())
  }, [])

  useEffect(() => {
    if (list === null) return

    // Whatever was already waiting when the app opened is history, not news. Without
    // this, coming back to a laptop left closed overnight would fire the whole backlog.
    if (!primed.current) {
      list.notices.forEach((n) => shown.current.add(keyOf(n)))
      primed.current = true
      return
    }

    // Someone looking at the app does not need the operating system to tell them.
    // It also covers the person who caused the event, who is by definition here.
    const watching = document.visibilityState === 'visible'

    for (const notice of [...list.notices].reverse()) {
      const key = keyOf(notice)
      if (shown.current.has(key)) continue

      shown.current.add(key)

      if (state === 'on' && !watching) {
        // Same tag as the server groups by, so a repeat replaces its popup instead
        // of stacking a second one beside it.
        new Notification('Deskbridge', { body: notice.line, tag: notice.kind })
      }
    }
  }, [list, state])

  return { state, ask }
}
