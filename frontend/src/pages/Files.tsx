import { useCallback, useRef, useState } from 'react'
import { api, uploadFile } from '../api/client'
import type { Shelf, SharedFile } from '../api/types'
import { shelves } from '../api/types'
import { usePoll } from '../hooks/usePoll'
import { useLiveFeed } from '../hooks/useLiveFeed'
import { AsyncPanel } from '../components/AsyncPanel'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { sizeLabel, dayLabel } from '../lib'

type Verdict = { intact: boolean; at: number }

export function FilesPage() {
  const [shelf, setShelf] = useState<Shelf | 'all'>('all')

  const fetchFiles = useCallback(
    () => api.files(shelf === 'all' ? undefined : shelf),
    [shelf],
  )

  const files = usePoll<SharedFile[]>(fetchFiles, 8000)

  const feed = useLiveFeed({
    'file.added': files.refresh,
    'file.removed': files.refresh,
  })

  const [dropping, setDropping] = useState(false)
  const [sending, setSending] = useState<string | null>(null)
  const [progress, setProgress] = useState(0)
  const [problem, setProblem] = useState<string | null>(null)
  const [verdicts, setVerdicts] = useState<Record<string, Verdict>>({})
  const picker = useRef<HTMLInputElement>(null)

  const send = async (chosen: File) => {
    const landing: Shelf = shelf === 'all' ? shelfFor(chosen.name) : shelf

    setSending(chosen.name)
    setProgress(0)
    setProblem(null)

    try {
      const { done } = uploadFile(chosen, landing, setProgress)
      await done
      files.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not upload')
    } finally {
      setSending(null)
      setProgress(0)
    }
  }

  const sendAll = async (list: FileList | null) => {
    if (!list) return
    for (const chosen of Array.from(list)) {
      await send(chosen)
    }
  }

  const remove = async (file: SharedFile) => {
    setProblem(null)
    try {
      await api.deleteFile(file.id)
      files.refresh()
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'that did not delete')
    }
  }

  const check = async (file: SharedFile) => {
    setProblem(null)
    try {
      const result = await api.verifyFile(file.id)
      setVerdicts((seen) => ({ ...seen, [file.id]: { intact: result.intact, at: Date.now() } }))
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'could not check that file')
    }
  }

  return (
    <div className="stack">
      <div className="row">
        <h1>Shared files</h1>
        <div className="row" style={{ gap: '.5rem' }}>
          {feed !== 'live' && <Tag tone="quiet">Polling only</Tag>}
          <Button small onClick={() => picker.current?.click()} disabled={sending !== null}>
            Choose a file
          </Button>
        </div>
      </div>

      <Card>
        <CardHeading>Drop zone</CardHeading>
        <Rule />

        <div
          className={dropping ? 'dropzone dropzone--armed' : 'dropzone'}
          onDragOver={(event) => {
            event.preventDefault()
            setDropping(true)
          }}
          onDragLeave={() => setDropping(false)}
          onDrop={(event) => {
            event.preventDefault()
            setDropping(false)
            sendAll(event.dataTransfer.files)
          }}
          onClick={() => picker.current?.click()}
        >
          {sending === null ? (
            <>
              <div style={{ fontWeight: 700 }}>Drag a file here</div>
              <div className="mono">or click to pick one &middot; up to 25 MB</div>
            </>
          ) : (
            <>
              <div style={{ fontWeight: 700 }}>Sending {sending}</div>
              <div className="meter">
                <span style={{ width: `${Math.round(progress * 100)}%` }} />
              </div>
              <div className="mono">{Math.round(progress * 100)}%</div>
            </>
          )}
        </div>

        <input
          ref={picker}
          type="file"
          multiple
          className="visually-hidden"
          onChange={(event) => {
            sendAll(event.target.files)
            event.target.value = ''
          }}
        />

        {problem && <div className="state state--error">{problem}</div>}
      </Card>

      <Card>
        <div className="row">
          <CardHeading>Shelves</CardHeading>
          <div className="shelf-tabs">
            {(['all', ...shelves] as const).map((name) => (
              <button
                key={name}
                type="button"
                className={shelf === name ? 'shelf shelf--picked' : 'shelf'}
                onClick={() => setShelf(name)}
              >
                {name}
              </button>
            ))}
          </div>
        </div>

        <Rule />

        <AsyncPanel
          poll={files}
          isEmpty={(list) => list.length === 0}
          empty="nothing on this shelf yet"
        >
          {(list) => (
            <div className="stack" style={{ gap: '.35rem' }}>
              {list.map((file) => {
                const verdict = verdicts[file.id]

                return (
                  <div className="row list-row" key={file.id}>
                    <div>
                      <div style={{ fontWeight: 700 }}>{file.name}</div>
                      <Tag tone={verdict === undefined ? 'quiet' : verdict.intact ? 'ok' : 'live'}>
                        {verdict === undefined
                          ? `${file.category} · ${sizeLabel(file.size_bytes)} · ${dayLabel(file.created_at)}`
                          : verdict.intact
                            ? 'Checked, matches its checksum'
                            : 'Checked, the bytes have changed'}
                      </Tag>
                    </div>
                    <div className="row" style={{ gap: '.4rem' }}>
                      <a className="btn btn--small" href={api.downloadURL(file.id)} download={file.name}>
                        Download
                      </a>
                      <Button small onClick={() => check(file)}>
                        Check
                      </Button>
                      <Button small onClick={() => remove(file)}>
                        Remove
                      </Button>
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

const shelfByExtension: Record<string, Shelf> = {
  png: 'images', jpg: 'images', jpeg: 'images', gif: 'images', webp: 'images', heic: 'images',
  pdf: 'documents', doc: 'documents', docx: 'documents', ppt: 'documents', pptx: 'documents',
  md: 'notes', txt: 'notes', rtf: 'notes',
  zip: 'resources', csv: 'resources', xlsx: 'resources',
}

// A guess, not a rule: the shelf tabs are right there if it lands in the wrong place.
function shelfFor(name: string): Shelf {
  const extension = name.split('.').pop()?.toLowerCase() ?? ''
  return shelfByExtension[extension] ?? 'other'
}
