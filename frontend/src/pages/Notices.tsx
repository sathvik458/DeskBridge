import { useState } from 'react'
import { api } from '../api/client'
import type { NoticeList, NoticeSettings } from '../api/types'
import type { Poll } from '../hooks/usePoll'
import { usePoll } from '../hooks/usePoll'
import type { AlertState } from '../hooks/useDesktopNotices'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { sinceLabel } from '../lib'

const names: Record<string, string> = {
  'help.raised': 'Help requests',
  'message.sent': 'Messages',
  'file.added': 'Shared files',
  'session.started': 'Study starting',
  'session.ended': 'Study finishing',
  'focus.changed': 'Focus stretches',
}

const shortNames: Record<string, string> = {
  'help.raised': 'Help',
  'message.sent': 'Message',
  'file.added': 'File',
  'session.started': 'Session',
  'session.ended': 'Session',
  'focus.changed': 'Focus',
}

interface NoticesPageProps {
  feed: Poll<NoticeList>
  alerts: { state: AlertState; ask: () => Promise<void> }
  now: number
}

export function NoticesPage({ feed, alerts, now }: NoticesPageProps) {
  const settings = usePoll<NoticeSettings>(api.noticeSettings, 60000)
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const run = async (action: () => Promise<unknown>, after: () => void) => {
    setBusy(true)
    setProblem(null)
    try {
      await action()
      after()
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
          <Button small disabled={busy} onClick={() => run(api.markNoticesSeen, feed.refresh)}>
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
                      {shortNames[notice.kind] ?? 'Update'}
                    </Tag>{' '}
                    {notice.line}
                  </div>
                  <span className="mono muted">{sinceLabel(notice.made_at, now)}</span>
                </div>
              ))}
            </div>
          )}
        </AsyncPanel>
      </Card>

      <Card alt>
        <CardHeading>Desktop alerts</CardHeading>
        <Rule />
        <AlertRow state={alerts.state} ask={alerts.ask} />
      </Card>

      <Card>
        <CardHeading>Tell me about</CardHeading>
        <Rule />

        <AsyncPanel poll={settings} isEmpty={(s) => s.switches.length === 0}>
          {(current) => (
            <div className="stack" style={{ gap: '.4rem' }}>
              {current.switches.map((sw) => (
                <label className="list-row" key={sw.kind}>
                  <span>{names[sw.kind] ?? sw.kind}</span>
                  <input
                    type="checkbox"
                    checked={sw.on}
                    disabled={busy}
                    onChange={(event) =>
                      run(
                        () => api.setNoticeSwitch(sw.kind, event.target.checked),
                        settings.refresh,
                      )
                    }
                  />
                </label>
              ))}
            </div>
          )}
        </AsyncPanel>

        <div className="mono muted" style={{ marginTop: '.6rem' }}>
          switching something off stops the notice being written at all, so it will not
          be waiting for you later either
        </div>
      </Card>

      {problem && <div className="state state--error">{problem}</div>}
    </div>
  )
}

// The browser only accepts a permission request that came from a click, so this is
// a button and never something that fires on its own when the page loads.
function AlertRow({ state, ask }: { state: AlertState; ask: () => Promise<void> }) {
  if (state === 'unsupported') {
    return <div className="state">this browser cannot show desktop alerts</div>
  }

  if (state === 'blocked') {
    return (
      <div className="state">
        alerts are blocked for this site &mdash; your browser settings are the only place
        that can undo it
      </div>
    )
  }

  if (state === 'on') {
    return (
      <div className="row">
        <Tag tone="ok">On</Tag>
        <span className="mono muted">only while this tab is in the background</span>
      </div>
    )
  }

  return (
    <div className="row">
      <span className="mono muted">off &mdash; nothing shows unless this tab is open</span>
      <Button small onClick={ask}>
        Turn on
      </Button>
    </div>
  )
}
