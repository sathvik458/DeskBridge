import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { FocusKind, FocusRun, FocusState, Session } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { useLiveFeed } from '../hooks/useLiveFeed'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { countdown, dots, kindLabel, secondsLeft, swept } from '../focus'

function isRunning(state: FocusState): state is FocusRun {
  return state.running
}

export function FocusPage({ now }: { now: number }) {
  const state = usePoll<FocusState>(api.focus, 15000)
  const session = usePoll<Session | null>(api.currentSession, 15000)
  const feed = useLiveFeed({ 'focus.changed': state.refresh })

  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const rang = useRef<string | null>(null)

  const run = state.data && isRunning(state.data) ? state.data : null
  const left = run ? secondsLeft(run.left_seconds, run.server_time, now) : 0

  // When the countdown hits zero the browser tells the server, once per run, so a
  // stretch that ran out while nobody was looking still gets recorded.
  useEffect(() => {
    if (!run || left > 0 || rang.current === run.id) return

    rang.current = run.id
    api
      .finishFocus(run.id)
      .catch(() => undefined)
      .finally(state.refresh)
  }, [run, left, state.refresh])

  const act = async (work: () => Promise<unknown>) => {
    setBusy(true)
    setProblem(null)
    try {
      await work()
      state.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not work')
    } finally {
      setBusy(false)
    }
  }

  // A stretch started while a session is running belongs to it, so the two line up
  // later rather than being two unrelated timers.
  const begin = (kind: FocusKind) =>
    act(() => api.startFocus(kind, kind === 'focus' ? (session.data?.id ?? null) : null))

  return (
    <div className="stack">
      <div className="row">
        <h1>Focus</h1>
        {feed !== 'live' && <Tag tone="quiet">Polling only</Tag>}
      </div>

      <AsyncPanel poll={state}>
        {(data) => (
          <div className="stack">
            <Card>
              <div className="row">
                <CardHeading>{run ? kindLabel(run.kind) : 'Ready'}</CardHeading>
                <div className="row" style={{ gap: '.3rem' }}>
                  {dots(data.into_round).map((filled, i) => (
                    <span key={i} className={filled ? 'pip pip--filled' : 'pip'} />
                  ))}
                </div>
              </div>
              <Rule />

              <div className="dial">
                <Ring swept={run ? swept(left, run.planned_seconds) : 0} />
                <div className="dial__face">
                  <div className="dial__time">
                    {run ? countdown(left) : countdown(0)}
                  </div>
                  <div className="mono">
                    {run ? 'running' : `next up: ${kindLabel(data.up_next).toLowerCase()}`}
                  </div>
                </div>
              </div>

              <div className="row" style={{ justifyContent: 'center', gap: '.5rem', marginTop: '.9rem' }}>
                {run ? (
                  <>
                    <Button weight="hatch" disabled={busy} onClick={() => act(() => api.finishFocus(run.id))}>
                      Done
                    </Button>
                    <Button disabled={busy} onClick={() => act(() => api.stopFocus(run.id))}>
                      Give up
                    </Button>
                  </>
                ) : (
                  <>
                    <Button weight="hatch" disabled={busy} onClick={() => begin(data.up_next)}>
                      Start {kindLabel(data.up_next).toLowerCase()}
                    </Button>
                    {data.up_next !== 'focus' && (
                      <Button disabled={busy} onClick={() => begin('focus')}>
                        Skip the break
                      </Button>
                    )}
                  </>
                )}
              </div>

              {problem && <div className="state state--error">{problem}</div>}
            </Card>

            <Card tight>
              <div className="row">
                <span className="mono">finished today</span>
                <span style={{ fontWeight: 700 }}>{data.done_today}</span>
              </div>
              {session.data && (
                <div className="row" style={{ marginTop: '.35rem' }}>
                  <span className="mono">counting towards</span>
                  <span style={{ fontWeight: 700 }}>{session.data.subject}</span>
                </div>
              )}
            </Card>
          </div>
        )}
      </AsyncPanel>
    </div>
  )
}

function Ring({ swept }: { swept: number }) {
  const r = 46
  const round = 2 * Math.PI * r

  return (
    <svg viewBox="0 0 110 110" className="dial__ring" aria-hidden="true">
      <circle cx="55" cy="55" r={r} className="dial__track" />
      <circle
        cx="55"
        cy="55"
        r={r}
        className="dial__sweep"
        strokeDasharray={round}
        strokeDashoffset={round * (1 - swept)}
        transform="rotate(-90 55 55)"
      />
    </svg>
  )
}
