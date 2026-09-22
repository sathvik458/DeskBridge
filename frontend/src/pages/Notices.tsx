import { useState } from 'react'
import { api } from '../api/client'
import type { NoticeList } from '../api/types'
import type { Poll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { sinceLabel } from '../lib'

const headings: Record<string, string> = {
  'help.raised': 'Help',
  'message.sent': 'Message',
  'file.added': 'File',
  'session.started': 'Session',
  'session.ended': 'Session',
  'focus.changed': 'Focus',
}

interface NoticesPageProps {
  feed: Poll<NoticeList>
  now: number
}

export function NoticesPage({ feed, now }: NoticesPageProps) {
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const clear = async () => {
    setBusy(true)
    setProblem(null)
    try {
      await api.markNoticesSeen()
      feed.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not work')
    } finally {
      setBusy(false)
    }
  }

  const unseen = feed.data?.unseen ?? 0

  return (
    <div className="stack">
      <div className="row">
        <h1>What you missed</h1>
        {unseen > 0 && (
          <Button small disabled={busy} onClick={clear}>
            Mark all seen
          </Button>
        )}
      </div>

      <Card>
        <CardHeading>Notices</CardHeading>
        <Rule />

        <AsyncPanel
          poll={feed}
          isEmpty={(list) => list.notices.length === 0}
          empty="nothing has happened while you were away"
        >
          {(list) => (
            <div className="stack" style={{ gap: '.4rem' }}>
              {list.notices.map((notice) => (
                <div className="list-row" key={notice.id}>
                  <div>
                    <Tag tone={notice.seen ? 'quiet' : 'live'}>
                      {headings[notice.kind] ?? 'Update'}
                    </Tag>{' '}
                    {notice.line}
                  </div>
                  <span className="mono muted">{sinceLabel(notice.made_at, now)}</span>
                </div>
              ))}
            </div>
          )}
        </AsyncPanel>

        {problem && <div className="state state--error">{problem}</div>}
      </Card>
    </div>
  )
}
