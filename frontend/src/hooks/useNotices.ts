import { useCallback } from 'react'
import { api } from '../api/client'
import type { NoticeList } from '../api/types'
import { usePoll } from './usePoll'
import { useLiveFeed } from './useLiveFeed'

const kinds = [
  'help.raised',
  'message.sent',
  'file.added',
  'session.started',
  'session.ended',
  'focus.changed',
]

// The live feed says something happened; this asks the server what it was. The
// row is what the bell counts, so a missed event costs a few seconds, not a notice.
export function useNotices(everyMs = 20000) {
  const poll = usePoll<NoticeList>(api.notices, everyMs)

  const nudge = useCallback(() => poll.refresh(), [poll.refresh])
  useLiveFeed(Object.fromEntries(kinds.map((kind) => [kind, nudge])))

  return poll
}
