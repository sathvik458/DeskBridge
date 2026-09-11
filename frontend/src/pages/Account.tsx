import { useState } from 'react'
import { api, RequestFailed } from '../api/client'
import type { Me } from '../api/types'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'

export function AccountPage({ me }: { me: Me }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const [done, setDone] = useState(false)

  const mismatch = again !== '' && next !== again

  const change = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy || mismatch) return

    setBusy(true)
    setProblem(null)
    setDone(false)

    try {
      await api.changePassword(current, next)
      setCurrent('')
      setNext('')
      setAgain('')
      setDone(true)
    } catch (err) {
      if (err instanceof RequestFailed && err.status === 401) {
        setProblem('that is not your current password')
      } else {
        setProblem(err instanceof Error ? err.message : 'that did not work')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="stack">
      <h1>Account</h1>

      <Card>
        <CardHeading>Who you are</CardHeading>
        <Rule />
        <div className="row list-row">
          <div>
            <div style={{ fontWeight: 700 }}>{me.display_name}</div>
            <Tag tone="quiet">signed in as {me.username}</Tag>
          </div>
          <div className="mono">{me.role}</div>
        </div>
      </Card>

      <Card>
        <CardHeading>Change your password</CardHeading>
        <Rule />

        <form className="stack" style={{ gap: '.9rem', maxWidth: '22rem' }} onSubmit={change}>
          <label className="field stack" style={{ gap: '.2rem' }}>
            <span className="mono">current password</span>
            <input
              type="password"
              value={current}
              autoComplete="current-password"
              onChange={(e) => setCurrent(e.target.value)}
            />
          </label>

          <label className="field stack" style={{ gap: '.2rem' }}>
            <span className="mono">new password (10 characters or more)</span>
            <input
              type="password"
              value={next}
              autoComplete="new-password"
              onChange={(e) => setNext(e.target.value)}
            />
          </label>

          <label className="field stack" style={{ gap: '.2rem' }}>
            <span className="mono">and again</span>
            <input
              type="password"
              value={again}
              autoComplete="new-password"
              onChange={(e) => setAgain(e.target.value)}
            />
          </label>

          {mismatch && <div className="state state--error">those two do not match</div>}

          <div>
            <Button
              type="submit"
              weight="hatch"
              disabled={busy || mismatch || current === '' || next === '' || again === ''}
            >
              {busy ? 'changing…' : 'Change it'}
            </Button>
          </div>

          {done && (
            <div className="state">
              done &mdash; every other browser has been signed out, this one is still fine
            </div>
          )}
          {problem && <div className="state state--error">{problem}</div>}
        </form>
      </Card>
    </div>
  )
}
