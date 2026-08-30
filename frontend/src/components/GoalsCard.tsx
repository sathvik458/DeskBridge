import { useState } from 'react'
import { api } from '../api/client'
import type { Goal } from '../api/types'
import type { Poll } from '../hooks/usePoll'
import { AsyncPanel } from './AsyncPanel'
import { Card, CardHeading, Rule } from './Card'
import { Button } from './Button'
import { Tag } from './Tag'

interface GoalsCardProps {
  poll: Poll<Goal[]>
  date: string
  showDelete?: boolean
}

export function GoalsCard({ poll, date, showDelete }: GoalsCardProps) {
  const [subject, setSubject] = useState('')
  const [topic, setTopic] = useState('')
  const [minutes, setMinutes] = useState('45')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

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

  const goals = poll.data ?? []
  const done = goals.filter((goal) => goal.done).length

  return (
    <Card>
      <div className="row">
        <CardHeading>Goals</CardHeading>
        {goals.length > 0 && (
          <Tag tone={done === goals.length ? 'ok' : 'note'}>
            {done} of {goals.length} done
          </Tag>
        )}
      </div>

      <Rule />

      <AsyncPanel poll={poll} isEmpty={(list) => list.length === 0} empty="nothing planned for this day">
        {(list) => (
          <div className="stack" style={{ gap: '.4rem' }}>
            {list.map((goal) => (
              <div className="row list-row" key={goal.id}>
                <label style={{ display: 'flex', alignItems: 'center', gap: '.6rem', cursor: 'pointer' }}>
                  <input
                    type="checkbox"
                    checked={goal.done}
                    disabled={busy}
                    onChange={() =>
                      run(() => (goal.done ? api.reopenGoal(goal.id) : api.completeGoal(goal.id)))
                    }
                  />
                  <span className={goal.done ? 'struck muted' : undefined}>
                    {goal.subject}
                    {goal.topic && <span className="muted"> — {goal.topic}</span>}
                  </span>
                </label>
                <div style={{ display: 'flex', alignItems: 'center', gap: '.5rem' }}>
                  <span className="mono">{goal.target_minutes}m</span>
                  {showDelete && (
                    <Button small disabled={busy} onClick={() => run(() => api.deleteGoal(goal.id))}>
                      Remove
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </AsyncPanel>

      <Rule />

      <form
        style={{ display: 'flex', gap: '.6rem', flexWrap: 'wrap', alignItems: 'center' }}
        onSubmit={(event) => {
          event.preventDefault()
          const target = Number(minutes)
          if (subject.trim() === '' || !Number.isFinite(target) || target < 1) return

          run(() => api.createGoal(subject.trim(), topic.trim() || null, target, date)).then(() => {
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
        <label className="field field--short">
          <span className="visually-hidden">Target minutes</span>
          <input
            type="number"
            min={1}
            max={1440}
            value={minutes}
            onChange={(e) => setMinutes(e.target.value)}
            aria-label="Target minutes"
          />
        </label>
        <Button type="submit" weight="hatch" disabled={busy}>
          Add goal
        </Button>
      </form>

      {problem && <div className="state state--error">{problem}</div>}
    </Card>
  )
}
