import type { ReactNode } from 'react'

interface CardProps {
  children: ReactNode
  tight?: boolean
  alt?: boolean
  accent?: boolean
}

export function Card({ children, tight, alt, accent }: CardProps) {
  const classes = ['sketch']
  if (tight) classes.push('sketch--tight')
  if (alt) classes.push('sketch--alt')
  if (accent) classes.push('sketch--accent')

  return <section className={classes.join(' ')}>{children}</section>
}

export function CardHeading({ children }: { children: ReactNode }) {
  return <div className="label">{children}</div>
}

export function Rule() {
  return <hr className="rule" />
}
