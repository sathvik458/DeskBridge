import { Card, CardHeading, Rule } from '../components/Card'

const audiences = [
  ['Parents working away from home', 'Stay part of your child’s study routine across a time zone.'],
  ['Relatives helping out', 'An aunt, uncle or older sibling who is good at maths and lives elsewhere.'],
  ['Tuition teachers with remote students', 'Run structured sessions, set homework, track hours, keep a record per student.'],
]

const features = [
  ['Desk view', 'an old phone becomes the camera'],
  ['Study sessions', 'start, pause, resume — timed by the server'],
  ['Daily goals', 'subject, topic, target minutes, progress'],
  ['Messages and help requests', 'the student asks, you get told'],
  ['Shared whiteboard', 'draw the diagram, they see it appear'],
  ['Files', 'worksheets, notes, photos of homework'],
  ['Focus mode', 'Pomodoro cycles, synced on both screens'],
  ['Study statistics', 'hours by subject, by day, by week'],
]

export function InfoPage() {
  return (
    <div className="stack">
      <div>
        <h1>
          Be there for a student,
          <br />
          from anywhere.
        </h1>
        <p style={{ fontSize: '1.1rem', maxWidth: '52ch', marginTop: '.6rem' }}>
          Deskbridge is a private study space you run on your own hardware. See the desk, keep time,
          set goals, answer questions, and work through a problem together — without either of you
          leaving home.
        </p>
      </div>

      <Card>
        <CardHeading>Who it’s for</CardHeading>
        <Rule />
        <div className="stack" style={{ gap: '.85rem' }}>
          {audiences.map(([who, what]) => (
            <div key={who}>
              <strong>{who}</strong>
              <div className="muted">{what}</div>
            </div>
          ))}
        </div>
      </Card>

      <Card>
        <CardHeading>What it does</CardHeading>
        <Rule />
        <div className="stack" style={{ gap: '.5rem' }}>
          {features.map(([name, detail]) => (
            <div key={name}>
              <strong>{name}</strong> <span className="muted">— {detail}</span>
            </div>
          ))}
        </div>
      </Card>

      <Card accent>
        <CardHeading>What it is not</CardHeading>
        <Rule />
        <p style={{ margin: '.2rem 0' }}>
          Deskbridge is not monitoring software. There is no attention scoring, no face or emotion
          detection, no hidden recording, and no report on a person’s behaviour. The camera shows a
          desk, and it says so loudly whenever it is on. Help is <em>asked for</em> by the student —
          never extracted.
        </p>
        <p className="muted" style={{ margin: '.2rem 0' }}>
          Both people should be comfortable with every screen. That is the design rule.
        </p>
      </Card>

      <Card>
        <CardHeading>Yours, on your own hardware</CardHeading>
        <Rule />
        <p style={{ margin: '.2rem 0' }}>
          Deskbridge runs on a computer you already own — an old laptop or desktop is plenty. Nothing
          is stored on anyone else’s servers. Study records, photographs and messages stay on your
          machine, and the two ends connect over a private network rather than the open internet.
        </p>
        <div className="mono" style={{ marginTop: '.5rem' }}>
          windows · macos · linux &nbsp;|&nbsp; go · sqlite · react
        </div>
      </Card>
    </div>
  )
}
