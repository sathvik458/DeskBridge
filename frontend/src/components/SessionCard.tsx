import { useState } from 'react'
import { api } from '../api/client'
import type { Session } from '../api/types'
import type { Poll } from '../hooks/usePoll'
import { Card, CardHeading, Rule } from './Card'
import { Button } from './Button'
import { Tag } from './Tag'
import { formatDuration, liveElapsed } from '../lib'

interface SessionCardProps {
  poll: Poll<Session | null>
  now: number
}

export function SessionCard({ poll, now }: SessionCardProps) {
  const [subject, setSubject] = useState('')
  const [topic, setTopic] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const session = poll.data

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    setProblem(null)
    try {
      await action()
      poll.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not work')
    } finally {
      setBusy(false)
    }
  }

  if (poll.loading && session === null) {
    return (
      <Card>
        <CardHeading>Current session</CardHeading>
        <div className="state">loading&hellip;</div>
      </Card>
    )
  }

  if (session === null) {
    return (
      <Card>
        <div className="row">
          <CardHeading>Current session</CardHeading>
          <Tag tone="quiet">Nothing running</Tag>
        </div>
        <Rule />
        <form
          style={{ display: 'flex', gap: '.6rem', flexWrap: 'wrap', alignItems: 'center' }}
          onSubmit={(event) => {
            event.preventDefault()
            if (subject.trim() === '') return
            run(() => api.startSession(subject.trim(), topic.trim() || null)).then(() => {
              setSubject('')
              setTopic('')
            })
          }}
        >
          <label className="field">
            <span className="visually-hidden">Subject</span>
            <input value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="Subject" required />
          </label>
          <label className="field">
            <span className="visually-hidden">Topic</span>
            <input value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="Topic (optional)" />
          </label>
          <Button type="submit" weight="hatch" disabled={busy}>
            Start studying
          </Button>
        </form>
        {problem && <div className="state state--error">{problem}</div>}
      </Card>
    )
  }

  const running = session.status === 'active'

  return (
    <Card>
      <div className="row">
        <CardHeading>Current session</CardHeading>
        <Tag tone={running ? 'ok' : 'quiet'}>{running ? 'Running' : 'Paused'}</Tag>
      </div>

      <div className="row" style={{ marginTop: '.4rem' }}>
        <div>
          <h2 style={{ marginBottom: '-.15rem' }}>{session.subject}</h2>
          <div className="muted">{session.topic ?? 'no topic set'}</div>
        </div>
        <div className="timer">{formatDuration(liveElapsed(session, now))}</div>
      </div>

      <Rule />

      <div style={{ display: 'flex', gap: '.6rem', flexWrap: 'wrap' }}>
        {running ? (
          <Button weight="ink" disabled={busy} onClick={() => run(() => api.pauseSession(session.id))}>
            Pause
          </Button>
        ) : (
          <Button weight="ink" disabled={busy} onClick={() => run(() => api.resumeSession(session.id))}>
            Resume
          </Button>
        )}
        <Button disabled={busy} onClick={() => run(() => api.endSession(session.id))}>
          End session
        </Button>
      </div>

      {problem && <div className="state state--error">{problem}</div>}
    </Card>
  )
}
