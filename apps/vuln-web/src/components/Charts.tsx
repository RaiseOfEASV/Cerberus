// Plain SVG charts - no chart library. Every value is also in the text
// (figcaption list) for screen readers and for anyone printing the page.

export function BarChart({ title, data, colors = {} }: { title: string; data: [string, number][]; colors?: Record<string, string> }) {
  const max = Math.max(1, ...data.map(([, v]) => v))
  const barH = 22
  const h = data.length * (barH + 8)
  return (
    <figure className="panel chart" aria-label={title}>
      <h2>{title}</h2>
      <svg role="img" aria-hidden="true" viewBox={`0 0 320 ${h}`} width="100%">
        {data.map(([label, v], i) => (
          <g key={label} transform={`translate(0 ${i * (barH + 8)})`}>
            <text x="0" y={barH * 0.7} fontSize="12">{label}</text>
            <rect x="110" y="0" height={barH} width={Math.max(2, (v / max) * 180)} className={colors[label] ?? 'bar'} />
            <text x={116 + (v / max) * 180} y={barH * 0.7} fontSize="12">{v}</text>
          </g>
        ))}
      </svg>
      <figcaption>
        <ul className="chart-values">
          {data.map(([label, v]) => <li key={label}>{label}: {v}</li>)}
        </ul>
      </figcaption>
    </figure>
  )
}

export function TrendChart({ title, weeks }: { title: string; weeks: { weekStart: string; opened: number; closed: number }[] }) {
  const max = Math.max(1, ...weeks.flatMap((w) => [w.opened, w.closed]))
  const W = 40
  const H = 120
  return (
    <figure className="panel chart" aria-label={title}>
      <h2>{title}</h2>
      <svg role="img" aria-hidden="true" viewBox={`0 0 ${weeks.length * W + 10} ${H + 20}`} width="100%">
        {weeks.map((w, i) => {
          const o = (w.opened / max) * H
          const c = (w.closed / max) * H
          return (
            <g key={w.weekStart} transform={`translate(${i * W + 5} 0)`}>
              <rect x="2" y={H - o} width="14" height={o} className="bar-opened" />
              <rect x="18" y={H - c} width="14" height={c} className="bar-closed" />
              <text x="2" y={H + 14} fontSize="9">{new Date(w.weekStart).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</text>
            </g>
          )
        })}
      </svg>
      <figcaption>
        <p><span className="key bar-opened" /> opened <span className="key bar-closed" /> closed</p>
        <ul className="chart-values">
          {weeks.map((w) => (
            <li key={w.weekStart}>week of {new Date(w.weekStart).toLocaleDateString()}: opened {w.opened}, closed {w.closed}</li>
          ))}
        </ul>
      </figcaption>
    </figure>
  )
}
