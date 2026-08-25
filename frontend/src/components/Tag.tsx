type Tone = 'ok' | 'live' | 'note' | 'quiet'

// Status is written out rather than shown as a coloured dot, so it reads without
// colour vision and says when something happened, not just whether it is true.
export function Tag({ tone = 'quiet', children }: { tone?: Tone; children: React.ReactNode }) {
  return <span className={`tag tag--${tone}`}>[ {children} ]</span>
}
