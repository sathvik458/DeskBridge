import { api } from '../api/client'
import type { Device, ServerStatus, Session, Goal } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Tag } from '../components/Tag'
import { SessionCard } from '../components/SessionCard'
import { GoalsCard } from '../components/GoalsCard'
import { todayISO } from '../lib'
import { deviceTag } from '../lib'

export function DashboardPage({ now }: { now: number }) {
  const status = usePoll<ServerStatus>(api.status, 10000)
  const devices = usePoll<Device[]>(api.devices, 4000)
  const session = usePoll<Session | null>(api.currentSession, 5000)
  const goals = usePoll<Goal[]>(api.goals, 15000)
  const today = todayISO()

  const unreachable = status.error !== null && status.data === null

  return (
    <div className="stack">
      {unreachable && (
        <div className="banner" role="alert">
          Cannot reach the Deskbridge server. Everything below is unavailable.
        </div>
      )}

      <SessionCard poll={session} now={now} />

      <GoalsCard poll={goals} date={today} />

      <Card>
        <div className="row">
          <CardHeading>Desk camera</CardHeading>
          <Tag tone="quiet">Camera not connected</Tag>
        </div>
        <div className="hatch-panel viewport">
          <div className="muted">no camera yet</div>
        </div>
      </Card>

      <Card>
        <div className="row">
          <CardHeading>Devices</CardHeading>
          <AsyncPanel poll={devices}>
            {(list) => (
              <Tag tone={list.some((d) => d.status === 'online') ? 'ok' : 'quiet'}>
                {list.filter((d) => d.status === 'online').length} of {list.length} online
              </Tag>
            )}
          </AsyncPanel>
        </div>
        <Rule />
        <AsyncPanel
          poll={devices}
          isEmpty={(list) => list.length === 0}
          empty="no devices have registered yet"
        >
          {(list) => (
            <div className="stack" style={{ gap: '.5rem' }}>
              {list.map((device) => {
                const tag = deviceTag(device, now)
                return (
                  <div key={device.id}>
                    <div style={{ fontWeight: 700 }}>{device.name}</div>
                    <Tag tone={tag.tone}>{tag.text}</Tag>
                  </div>
                )
              })}
            </div>
          )}
        </AsyncPanel>
      </Card>
    </div>
  )
}

export function ServerCard({ status }: { status: ReturnType<typeof usePoll<ServerStatus>> }) {
  return (
    <Card tight alt>
      <CardHeading>Server</CardHeading>
      <AsyncPanel poll={status}>
        {(data) => (
          <>
            <Tag tone="ok">Up {data.uptime}</Tag>
            <div className="mono" style={{ marginTop: '.15rem' }}>
              {data.version} · {data.platform}
            </div>
          </>
        )}
      </AsyncPanel>
    </Card>
  )
}
