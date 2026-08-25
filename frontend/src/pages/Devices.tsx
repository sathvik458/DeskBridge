import { api } from '../api/client'
import type { Device } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Tag } from '../components/Tag'
import { deviceTag } from '../lib'

export function DevicesPage({ now }: { now: number }) {
  const devices = usePoll<Device[]>(api.devices, 4000)

  return (
    <div className="stack">
      <h1>Devices</h1>

      <Card>
        <CardHeading>Registered devices</CardHeading>
        <Rule />
        <AsyncPanel
          poll={devices}
          isEmpty={(list) => list.length === 0}
          empty="no devices have registered yet"
        >
          {(list) => (
            <div className="stack" style={{ gap: '.35rem' }}>
              {list.map((device) => {
                const tag = deviceTag(device, now)
                return (
                  <div className="row list-row" key={device.id}>
                    <div>
                      <div style={{ fontWeight: 700 }}>{device.name}</div>
                      <Tag tone={tag.tone}>{tag.text}</Tag>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div className="mono">{device.kind}</div>
                      <div className="mono">{device.id}</div>
                    </div>
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
