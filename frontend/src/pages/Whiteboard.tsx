import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api/client'
import { pens } from '../api/types'
import type { Mark, Pen } from '../api/types'
import { useBoard } from '../hooks/useBoard'
import { useLiveFeed } from '../hooks/useLiveFeed'
import { Card, CardHeading, Rule } from '../components/Card'
import { Button } from '../components/Button'
import { Tag } from '../components/Tag'
import { paint, strokesFrom, strokeUnder } from '../board'

const nibs = [2, 4, 8]
const sampleGap = 0.004
const eraserReach = 0.02

export function WhiteboardPage() {
  const board = useBoard()
  const feed = useLiveFeed({ 'board.marked': board.pull })

  const [ink, setInk] = useState<Pen>(pens[0])
  const [nib, setNib] = useState(nibs[1])
  const [rubbing, setRubbing] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  const settled = useRef<HTMLCanvasElement>(null)
  const live = useRef<HTMLCanvasElement>(null)
  const drawing = useRef<number[] | null>(null)
  const [size, setSize] = useState({ width: 0, height: 0 })

  const strokes = useMemo(() => strokesFrom(board.marks), [board.marks])

  // The canvas has to be sized in device pixels or every line comes out soft on a
  // retina screen, while the drawing code keeps working in CSS pixels.
  const measure = useCallback(() => {
    const canvas = settled.current
    if (!canvas) return

    const box = canvas.getBoundingClientRect()
    const ratio = window.devicePixelRatio || 1

    for (const layer of [settled.current, live.current]) {
      if (!layer) continue
      layer.width = Math.round(box.width * ratio)
      layer.height = Math.round(box.height * ratio)
      layer.getContext('2d')?.setTransform(ratio, 0, 0, ratio, 0, 0)
    }

    setSize({ width: box.width, height: box.height })
  }, [])

  useEffect(() => {
    measure()
    window.addEventListener('resize', measure)
    return () => window.removeEventListener('resize', measure)
  }, [measure])

  useEffect(() => {
    const ctx = settled.current?.getContext('2d')
    if (!ctx || size.width === 0) return
    paint(ctx, strokes, size.width, size.height)
  }, [strokes, size])

  const spotOf = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const box = event.currentTarget.getBoundingClientRect()
    return {
      x: Math.min(1, Math.max(0, (event.clientX - box.left) / box.width)),
      y: Math.min(1, Math.max(0, (event.clientY - box.top) / box.height)),
    }
  }

  const send = async (action: () => Promise<Mark>) => {
    setProblem(null)
    try {
      board.absorb(await action())
    } catch (err) {
      setProblem(err instanceof Error ? err.message : 'the board did not take that')
      board.pull()
    }
  }

  const begin = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const spot = spotOf(event)

    if (rubbing) {
      const hit = strokeUnder(strokes, spot.x, spot.y, eraserReach)
      if (hit) send(() => api.eraseStroke(hit))
      return
    }

    event.currentTarget.setPointerCapture(event.pointerId)
    drawing.current = [spot.x, spot.y]
  }

  const extend = (event: React.PointerEvent<HTMLCanvasElement>) => {
    const path = drawing.current
    if (!path) return

    const spot = spotOf(event)
    const lastX = path[path.length - 2]
    const lastY = path[path.length - 1]

    // A pointer fires far faster than the drawing needs. Dropping points that land
    // almost on the previous one keeps a stroke small without changing its shape.
    if (Math.hypot(spot.x - lastX, spot.y - lastY) < sampleGap) return

    path.push(spot.x, spot.y)

    const ctx = live.current?.getContext('2d')
    if (ctx) {
      paint(ctx, [{ id: 'live', ink, thickness: nib, path }], size.width, size.height)
    }
  }

  const finish = () => {
    const path = drawing.current
    drawing.current = null

    live.current?.getContext('2d')?.clearRect(0, 0, size.width, size.height)

    if (!path || path.length < 2) return

    send(() => api.drawStroke(ink, nib, path))
  }

  return (
    <div className="stack">
      <div className="row">
        <h1>Whiteboard</h1>
        <div className="row" style={{ gap: '.5rem' }}>
          {feed !== 'live' && <Tag tone="quiet">Polling only</Tag>}
          <Button small onClick={() => setRubbing((on) => !on)}>
            {rubbing ? 'Stop rubbing out' : 'Rub out'}
          </Button>
          <Button small onClick={() => send(api.clearBoard)}>
            Clear
          </Button>
        </div>
      </div>

      <Card>
        <div className="row">
          <CardHeading>Board</CardHeading>
          <div className="row" style={{ gap: '.9rem' }}>
            <div className="pens">
              {pens.map((colour) => (
                <button
                  key={colour}
                  type="button"
                  aria-label={`pen ${colour}`}
                  className={ink === colour && !rubbing ? 'pen pen--picked' : 'pen'}
                  style={{ background: colour }}
                  onClick={() => {
                    setInk(colour)
                    setRubbing(false)
                  }}
                />
              ))}
            </div>
            <div className="pens">
              {nibs.map((width) => (
                <button
                  key={width}
                  type="button"
                  aria-label={`nib ${width}`}
                  className={nib === width ? 'nib nib--picked' : 'nib'}
                  onClick={() => setNib(width)}
                >
                  <span style={{ width: width + 2, height: width + 2 }} />
                </button>
              ))}
            </div>
          </div>
        </div>

        <Rule />

        <div className={rubbing ? 'board board--rubbing' : 'board'}>
          <canvas ref={settled} className="board__layer" />
          <canvas
            ref={live}
            className="board__layer board__layer--live"
            onPointerDown={begin}
            onPointerMove={extend}
            onPointerUp={finish}
            onPointerCancel={finish}
          />
        </div>

        <div className="row" style={{ marginTop: '.5rem' }}>
          <span className="mono">
            {board.loading ? 'loading…' : `${strokes.length} strokes on the board`}
          </span>
          {rubbing && <Tag tone="live">Tap a stroke to rub it out</Tag>}
        </div>

        {(problem ?? board.error) && (
          <div className="state state--error">{problem ?? board.error}</div>
        )}
      </Card>
    </div>
  )
}
