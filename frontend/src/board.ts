import type { Mark, Pen } from './api/types'

export interface Stroke {
  id: string
  ink: Pen
  thickness: number
  path: number[]
}

// The log is the truth and this is the picture. Replaying it beats mutating a
// drawing in place, because a mark that arrives late still lands in the right order.
export function strokesFrom(marks: Mark[]): Stroke[] {
  const rubbedOut = new Set<string>()
  let clearedUpTo = 0

  for (const mark of marks) {
    if (mark.kind === 'erase' && mark.target_id) rubbedOut.add(mark.target_id)
    if (mark.kind === 'clear') clearedUpTo = mark.seq
  }

  const visible: Stroke[] = []

  for (const mark of marks) {
    if (mark.kind !== 'draw') continue
    if (mark.seq < clearedUpTo) continue
    if (rubbedOut.has(mark.id)) continue
    if (!mark.path || !mark.ink || !mark.thickness) continue

    visible.push({ id: mark.id, ink: mark.ink, thickness: mark.thickness, path: mark.path })
  }

  return visible
}

// Squared distance from a point to a segment, without the square root - only the
// comparison matters and skipping it keeps the hit test cheap on long strokes.
function gapToSegment(px: number, py: number, ax: number, ay: number, bx: number, by: number) {
  const dx = bx - ax
  const dy = by - ay
  const lengthSquared = dx * dx + dy * dy

  if (lengthSquared === 0) {
    return (px - ax) ** 2 + (py - ay) ** 2
  }

  let along = ((px - ax) * dx + (py - ay) * dy) / lengthSquared
  along = Math.max(0, Math.min(1, along))

  const nearestX = ax + along * dx
  const nearestY = ay + along * dy

  return (px - nearestX) ** 2 + (py - nearestY) ** 2
}

export function strokeUnder(strokes: Stroke[], x: number, y: number, reach: number): string | null {
  const reachSquared = reach * reach
  let closest: string | null = null
  let closestGap = Infinity

  // Backwards, so clicking overlapping strokes picks the one drawn on top.
  for (let i = strokes.length - 1; i >= 0; i--) {
    const { path, id } = strokes[i]

    for (let p = 0; p + 3 < path.length; p += 2) {
      const gap = gapToSegment(x, y, path[p], path[p + 1], path[p + 2], path[p + 3])
      if (gap < closestGap) {
        closestGap = gap
        closest = id
      }
    }

    if (path.length === 2) {
      const gap = (x - path[0]) ** 2 + (y - path[1]) ** 2
      if (gap < closestGap) {
        closestGap = gap
        closest = id
      }
    }
  }

  return closestGap <= reachSquared ? closest : null
}

export function paint(
  ctx: CanvasRenderingContext2D,
  strokes: Stroke[],
  width: number,
  height: number,
) {
  ctx.clearRect(0, 0, width, height)
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'

  for (const stroke of strokes) {
    if (stroke.path.length < 2) continue

    ctx.strokeStyle = stroke.ink
    ctx.lineWidth = stroke.thickness
    ctx.beginPath()
    ctx.moveTo(stroke.path[0] * width, stroke.path[1] * height)

    for (let p = 2; p + 1 < stroke.path.length; p += 2) {
      ctx.lineTo(stroke.path[p] * width, stroke.path[p + 1] * height)
    }

    if (stroke.path.length === 2) {
      ctx.lineTo(stroke.path[0] * width + 0.01, stroke.path[1] * height)
    }

    ctx.stroke()
  }
}
