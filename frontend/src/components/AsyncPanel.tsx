import type { ReactNode } from 'react'
import type { Poll } from '../hooks/usePoll'

interface AsyncPanelProps<T> {
  poll: Poll<T>
  empty?: string
  isEmpty?: (data: T) => boolean
  children: (data: T) => ReactNode
}

// One place that decides what loading, failed, empty and stale look like, so no
// screen quietly forgets one of them.
export function AsyncPanel<T>({ poll, empty, isEmpty, children }: AsyncPanelProps<T>) {
  const { data, error, loading, stale } = poll

  if (loading && data === null) {
    return <div className="state">loading&hellip;</div>
  }

  if (error !== null && data === null) {
    return <div className="state state--error">could not load &mdash; {error}</div>
  }

  if (data === null) return null

  if (isEmpty?.(data)) {
    return <div className="state">{empty ?? 'nothing here yet'}</div>
  }

  return <div className={stale ? 'stale' : undefined}>{children(data)}</div>
}
