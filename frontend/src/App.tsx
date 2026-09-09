import { useEffect, useState } from 'react'
import { api } from './api/client'
import type { ServerStatus } from './api/types'
import { usePoll } from './hooks/usePoll'
import { SketchFilters } from './components/SketchFilters'
import { DashboardPage, ServerCard } from './pages/Dashboard'
import { DevicesPage } from './pages/Devices'
import { SessionsPage } from './pages/Sessions'
import { StudyPlanPage } from './pages/StudyPlan'
import { MessagesPage } from './pages/Messages'
import { FilesPage } from './pages/Files'
import { WhiteboardPage } from './pages/Whiteboard'
import { SignInPage } from './pages/SignIn'
import { useMe } from './hooks/useMe'
import { InfoPage } from './pages/Info'

const pages = ['Dashboard', 'Study Sessions', 'Study Plan', 'Whiteboard', 'Files', 'Messages', 'Devices', 'Info'] as const
type Page = (typeof pages)[number]

const planned = ['Settings']

export function App() {
  const who = useMe()
  const [page, setPage] = useState<Page>('Dashboard')
  const status = usePoll<ServerStatus>(api.status, 10000)

  // One clock for the whole app, so every "last seen" label re-renders together
  // instead of each row keeping its own timer.
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  if (who.state === 'asking') {
    return <div className="gateway mono">one moment&hellip;</div>
  }

  if (who.state === 'out' || who.me === null) {
    return <SignInPage onIn={who.arrived} />
  }

  return (
    <>
      <SketchFilters />
      <div className="shell">
        <aside className="sidebar stack" style={{ alignContent: 'start' }}>
          <div>
            <h1 style={{ marginBottom: '-.3rem' }}>Deskbridge</h1>
            <div className="mono">private study space</div>
          </div>

          <nav aria-label="Sections">
            <ul className="nav">
              {pages.map((name) => (
                <li key={name}>
                  <a
                    href={`#${name.toLowerCase()}`}
                    aria-current={page === name ? 'page' : undefined}
                    onClick={(event) => {
                      event.preventDefault()
                      setPage(name)
                    }}
                  >
                    {name}
                  </a>
                </li>
              ))}
              {planned.map((name) => (
                <li key={name}>
                  <a href="#" aria-disabled="true" className="muted" onClick={(e) => e.preventDefault()}>
                    {name}
                  </a>
                </li>
              ))}
            </ul>
          </nav>

          <div style={{ marginTop: '.6rem' }} className="stack">
            <ServerCard status={status} />
            <div className="row">
              <span className="mono">{who.me.display_name}</span>
              <button type="button" className="linkish mono" onClick={who.left}>
                sign out
              </button>
            </div>
          </div>
        </aside>

        <main>
          {page === 'Dashboard' && <DashboardPage now={now} />}
          {page === 'Study Sessions' && <SessionsPage />}
          {page === 'Study Plan' && <StudyPlanPage />}
          {page === 'Whiteboard' && <WhiteboardPage />}
          {page === 'Files' && <FilesPage />}
          {page === 'Messages' && <MessagesPage />}
          {page === 'Devices' && <DevicesPage now={now} />}
          {page === 'Info' && <InfoPage />}
        </main>
      </div>
    </>
  )
}
