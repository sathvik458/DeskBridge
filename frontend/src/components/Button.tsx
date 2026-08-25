import type { ButtonHTMLAttributes } from 'react'

type Weight = 'outline' | 'hatch' | 'ink'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  weight?: Weight
  small?: boolean
}

export function Button({ weight = 'outline', small, className, ...rest }: ButtonProps) {
  const classes = ['btn']
  if (weight === 'hatch') classes.push('btn--hatch')
  if (weight === 'ink') classes.push('btn--ink')
  if (small) classes.push('btn--small')
  if (className) classes.push(className)

  return <button className={classes.join(' ')} {...rest} />
}
