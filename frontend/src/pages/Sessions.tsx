import { api } from '../api/client'
import type { Session } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Tag } from '../components/Tag'
import { formatDuration } from '../lib'

function startedLabel(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function SessionsPage() {
  const sessions = usePoll<Session[]>(api.sessions, 8000)

  return (
    <div className="stack">
      <h1>Study Sessions</h1>

      <Card>
        <CardHeading>History</CardHeading>
        <Rule />
        <AsyncPanel
          poll={sessions}
          isEmpty={(list) => list.length === 0}
          empty="no sessions recorded yet"
        >
          {(list) => (
            <div className="stack" style={{ gap: '.4rem' }}>
              {list.map((session) => (
                <div className="row list-row" key={session.id}>
                  <div>
                    <div style={{ fontWeight: 700 }}>
                      {session.subject}
                      {session.topic && <span className="muted"> — {session.topic}</span>}
                    </div>
                    <Tag tone={session.status === 'completed' ? 'quiet' : 'ok'}>
                      {session.status} · started {startedLabel(session.started_at)}
                    </Tag>
                  </div>
                  <div className="mono" style={{ fontSize: '1rem' }}>
                    {formatDuration(session.elapsed_seconds)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </AsyncPanel>
      </Card>
    </div>
  )
}
