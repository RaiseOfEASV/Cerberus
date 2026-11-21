// Inline SVG icons (no icon font or CDN - the CSP allows only 'self').
const paths: Record<string, string> = {
  shield: 'M12 3l7 3v5c0 4.6-3 8.5-7 10-4-1.5-7-5.4-7-10V6l7-3z',
  grid: 'M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z',
  list: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01',
  plus: 'M12 5v14M5 12h14',
  upload: 'M12 16V4M7 9l5-5 5 5M5 20h14',
  history: 'M3 12a9 9 0 1 0 3-6.7L3 8M3 3v5h5M12 7v5l3 2',
  users: 'M16 19v-1a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v1M9 10a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM22 19v-1a4 4 0 0 0-3-3.9M16 4.1a3 3 0 0 1 0 5.8',
  clock: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 3',
  lock: 'M6 11h12v9H6zM8 11V8a4 4 0 0 1 8 0v3',
  inbox: 'M3 13l3-8h12l3 8v6H3zM3 13h5l1 2h6l1-2h5',
  chart: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
  check: 'M5 12l4 4 10-10',
  file: 'M14 3H6v18h12V7zM14 3v4h4',
  logout: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
  arrow: 'M5 12h14M13 6l6 6-6 6',
  scan: 'M4 8V4h4M16 4h4v4M20 16v4h-4M8 20H4v-4M4 12h16',
}

export default function Icon({ name, size = 18 }: { name: keyof typeof paths | string; size?: number }) {
  return (
    <svg className="icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={paths[name] ?? ''} />
    </svg>
  )
}
