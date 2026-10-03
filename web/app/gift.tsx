// The gift, in gold, in the corner of every screen.
//
// Gold is used here and nowhere else in the app. A colour that appears in
// exactly one place keeps meaning something; the same colour used twice
// becomes decoration, and then the corner stops being worth looking at.

export default function Gift({ size = 20 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="3" y="9.5" width="18" height="11.5" rx="2" fill="var(--gold)" />
      <rect x="2" y="6" width="20" height="4.5" rx="1.4" fill="var(--gold-ink)" />
      <rect x="10.4" y="6" width="3.2" height="15" fill="rgba(255,255,255,0.55)" />
      <path
        d="M12 6c-1.6-3-6-3-6-0.6C6 7 8.6 7.2 12 6zm0 0c1.6-3 6-3 6-0.6C18 7 15.4 7.2 12 6z"
        fill="var(--gold-ink)"
      />
    </svg>
  )
}
