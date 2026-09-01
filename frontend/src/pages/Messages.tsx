import { useState } from 'react'
import { api } from '../api/client'
import type { Message } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { timeLabel } from '../lib'

export function MessagesPage() {
  const messages = usePoll<Message[]>(api.messages, 5000)
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    setProblem(null)
    try {
      await action()
      messages.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not send')
    } finally {
      setBusy(false)
    }
  }

  const unreadFromStudent = (messages.data ?? []).filter((m) => !m.read && m.from === 'student')

  return (
    <div className="stack">
      <div className="row">
        <h1>Messages</h1>
        {unreadFromStudent.length > 0 && (
          <Button small disabled={busy} onClick={() => run(api.markAllMessagesRead)}>
            Mark all read
          </Button>
        )}
      </div>

      <Card>
        <CardHeading>Conversation</CardHeading>
        <Rule />

        <AsyncPanel poll={messages} isEmpty={(list) => list.length === 0} empty="no messages yet">
          {(list) => (
            <div className="thread">
              {/* The API returns newest first for the limit to be useful; a conversation
                  reads oldest first, so it is reversed here rather than in the query. */}
              {[...list].reverse().map((message) => {
                const mine = message.from === 'supporter'
                const classes = ['bubble', mine ? 'bubble--mine' : 'bubble--theirs']
                if (message.kind === 'help_request') classes.push('bubble--help')

                return (
                  <div className={classes.join(' ')} key={message.id}>
                    {message.kind === 'help_request' && <Tag tone="live">Help requested</Tag>}
                    <div>{message.body}</div>
                    <div className="mono">
                      {mine ? 'you' : 'student'} &middot; {timeLabel(message.created_at)}
                      {!message.read && !mine && ' · unread'}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </AsyncPanel>

        <Rule />

        <form
          className="composer stack"
          style={{ gap: '.6rem' }}
          onSubmit={(event) => {
            event.preventDefault()
            if (draft.trim() === '') return
            run(() => api.sendMessage('supporter', draft.trim())).then(() => setDraft(''))
          }}
        >
          <label>
            <span className="visually-hidden">Message</span>
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              placeholder="Write a message…"
            />
          </label>
          <div>
            <Button type="submit" weight="hatch" disabled={busy}>
              Send
            </Button>
          </div>
        </form>

        {problem && <div className="state state--error">{problem}</div>}
      </Card>
    </div>
  )
}
