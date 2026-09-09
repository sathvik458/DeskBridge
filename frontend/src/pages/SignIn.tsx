import { useState } from 'react'
import { api, RequestFailed } from '../api/client'
import type { Me } from '../api/types'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { SketchFilters } from '../components/SketchFilters'

export function SignInPage({ onIn }: { onIn: (me: Me) => void }) {
  const [name, setName] = useState('')
  const [word, setWord] = useState('')
  const [firstTime, setFirstTime] = useState(false)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const go = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return

    setBusy(true)
    setProblem(null)

    try {
      const label = `${navigator.platform || 'a device'} browser`
      const me = firstTime
        ? await api.claim(name, word, label)
        : await api.signIn(name, word, label)
      onIn(me)
    } catch (err) {
      // A 409 means somebody already set this account up, which is worth saying
      // plainly rather than showing as a generic failure.
      if (err instanceof RequestFailed && err.status === 409) {
        setProblem('that account is already set up — sign in instead')
        setFirstTime(false)
      } else {
        setProblem(err instanceof Error ? err.message : 'that did not work')
      }
      setWord('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <SketchFilters />
      <div className="gateway">
        <div className="stack" style={{ maxWidth: '25rem', width: '100%' }}>
          <div>
            <h1 style={{ marginBottom: '-.3rem' }}>Deskbridge</h1>
            <div className="mono">private study space</div>
          </div>

          <Card>
            <CardHeading>{firstTime ? 'Set up this account' : 'Sign in'}</CardHeading>
            <Rule />

            <form className="stack" style={{ gap: '.9rem' }} onSubmit={go}>
              <label className="field stack" style={{ gap: '.2rem' }}>
                <span className="mono">who are you</span>
                <input
                  value={name}
                  autoComplete="username"
                  autoFocus
                  onChange={(e) => setName(e.target.value)}
                  placeholder="student or supporter"
                />
              </label>

              <label className="field stack" style={{ gap: '.2rem' }}>
                <span className="mono">
                  {firstTime ? 'pick a password (10 characters or more)' : 'password'}
                </span>
                <input
                  type="password"
                  value={word}
                  autoComplete={firstTime ? 'new-password' : 'current-password'}
                  onChange={(e) => setWord(e.target.value)}
                />
              </label>

              <div className="row">
                <Button type="submit" weight="hatch" disabled={busy || name === '' || word === ''}>
                  {busy ? 'one moment…' : firstTime ? 'Set it up' : 'Sign in'}
                </Button>
                <button
                  type="button"
                  className="linkish mono"
                  onClick={() => {
                    setFirstTime((on) => !on)
                    setProblem(null)
                  }}
                >
                  {firstTime ? 'I already have a password' : 'first time on this server'}
                </button>
              </div>

              {problem && <div className="state state--error">{problem}</div>}
            </form>
          </Card>

          <div className="mono muted">
            Nothing ships with a password. The first person to reach a fresh server picks one.
          </div>
        </div>
      </div>
    </>
  )
}
