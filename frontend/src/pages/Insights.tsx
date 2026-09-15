import { useCallback, useState } from 'react'
import { api } from '../api/client'
import type { Insights } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Tag } from '../components/Tag'
import { hoursLabel, shortDay } from '../lib'

const spans = [7, 14, 30]

export function InsightsPage() {
  const [span, setSpan] = useState(14)

  const fetchInsights = useCallback(() => api.insights(span), [span])
  const report = usePoll<Insights>(fetchInsights, 30000)

  return (
    <div className="stack">
      <div className="row">
        <h1>Insights</h1>
        <div className="shelf-tabs">
          {spans.map((n) => (
            <button
              key={n}
              type="button"
              className={span === n ? 'shelf shelf--picked' : 'shelf'}
              onClick={() => setSpan(n)}
            >
              {n} days
            </button>
          ))}
        </div>
      </div>

      <AsyncPanel poll={report}>
        {(data) => (
          <div className="stack">
            <div className="tiles">
              <Figure label="total studied" value={hoursLabel(data.total_seconds)} />
              <Figure label="sessions" value={String(data.session_count)} />
              <Figure label="day streak" value={String(data.streak)} />
              <Figure
                label="goals done"
                value={data.goals_set === 0 ? '—' : `${data.goals_done}/${data.goals_set}`}
              />
            </div>

            <Card>
              <div className="row">
                <CardHeading>Time per day</CardHeading>
                {data.best_day.seconds > 0 && (
                  <Tag tone="ok">best {hoursLabel(data.best_day.seconds)}</Tag>
                )}
              </div>
              <Rule />
              <Bars data={data} />
            </Card>

            <Card>
              <CardHeading>By subject</CardHeading>
              <Rule />
              {data.subjects.length === 0 ? (
                <div className="state">nothing studied in this stretch yet</div>
              ) : (
                <div className="stack" style={{ gap: '.35rem' }}>
                  {data.subjects.map((s) => (
                    <div className="row list-row" key={s.name}>
                      <div style={{ fontWeight: 700 }}>{s.name}</div>
                      <div className="row" style={{ gap: '.7rem' }}>
                        <span className="mono">{s.sessions} sessions</span>
                        <span style={{ fontWeight: 700 }}>{hoursLabel(s.seconds)}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>

            <div className="mono muted">days counted in {data.zone}</div>
          </div>
        )}
      </AsyncPanel>
    </div>
  )
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div className="tile">
      <div className="tile__value">{value}</div>
      <div className="label">{label}</div>
    </div>
  )
}

// Plain SVG rather than a chart library: it is a row of rectangles, and the whole
// project has one dependency.
function Bars({ data }: { data: Insights }) {
  const tallest = Math.max(...data.days.map((d) => d.seconds), 1)
  const width = 100 / data.days.length

  return (
    <div className="chart">
      <svg viewBox="0 0 100 42" preserveAspectRatio="none" role="img" aria-label="study time per day">
        {data.days.map((d, i) => {
          const height = (d.seconds / tallest) * 36
          return (
            <rect
              key={d.date}
              x={i * width + width * 0.18}
              y={38 - height}
              width={width * 0.64}
              height={Math.max(height, d.seconds > 0 ? 0.8 : 0.25)}
              className={d.seconds > 0 ? 'bar' : 'bar bar--none'}
            >
              <title>
                {d.date} — {hoursLabel(d.seconds)} over {d.sessions} sessions
              </title>
            </rect>
          )
        })}
      </svg>

      <div className="chart__feet">
        {data.days.map((d) => (
          <span className="mono" key={d.date}>
            {shortDay(d.date)}
          </span>
        ))}
      </div>
    </div>
  )
}
