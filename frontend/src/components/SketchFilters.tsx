export function SketchFilters() {
  return (
    <svg width="0" height="0" style={{ position: 'absolute' }} aria-hidden="true">
      <filter id="rough">
        <feTurbulence type="fractalNoise" baseFrequency="0.022" numOctaves="4" seed="7" result="n" />
        <feDisplacementMap in="SourceGraphic" in2="n" scale="3.4" xChannelSelector="R" yChannelSelector="G" />
      </filter>
      <filter id="rough-soft">
        <feTurbulence type="fractalNoise" baseFrequency="0.04" numOctaves="3" seed="3" result="n" />
        <feDisplacementMap in="SourceGraphic" in2="n" scale="2" xChannelSelector="R" yChannelSelector="G" />
      </filter>
    </svg>
  )
}
