import { api } from '../api/client'
import type { Message } from '../api/types'
import type { Poll } from '../hooks/usePoll'
import { Card, CardHeading } from './Card'
import { Button } from './Button'
import { Tag } from './Tag'
import { timeLabel } from '../lib'

export function HelpBanner({ poll }: { poll: Poll<Message[]> }) {
  const unread = poll.data ?? []
  const help = unread.filter((message) => message.kind === 'help_request')

  if (help.length === 0) return null

  return (
    <Card accent>
      <div className="stack" style={{ gap: '.7rem' }}>
        {help.map((message) => (
          <div className="row" key={message.id} style={{ alignItems: 'flex-start' }}>
            <div>
              <Tag tone="live">Help requested &middot; {timeLabel(message.created_at)}</Tag>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, marginTop: '.25rem' }}>
                &ldquo;{message.body}&rdquo;
              </div>
            </div>
            <Button
              small
              weight="hatch"
              onClick={() => api.markMessageRead(message.id).then(poll.refresh)}
            >
              Seen
            </Button>
          </div>
        ))}
      </div>
    </Card>
  )
}

export function RecentMessages({ poll }: { poll: Poll<Message[]> }) {
  const messages = (poll.data ?? []).slice(0, 4)

  if (messages.length === 0) return null

  return (
    <Card tight alt>
      <CardHeading>Recent messages</CardHeading>
      <div className="stack" style={{ gap: '.35rem', marginTop: '.5rem' }}>
        {messages.map((message) => (
          <div className="row" key={message.id}>
            <span className={message.read ? 'muted' : undefined}>
              <strong>{message.from === 'supporter' ? 'You' : 'Student'}:</strong> {message.body}
            </span>
            <span className="mono">{timeLabel(message.created_at)}</span>
          </div>
        ))}
      </div>
    </Card>
  )
}
