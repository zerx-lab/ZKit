import { cn } from "@/lib/utils";

/**
 * BrandLogo — the ZKit mark.
 *
 * A rounded-square tile in the brand blue with a stylized "z" whose lower
 * stroke extends into a baseline bar, evoking a lab bench / toolkit shelf.
 * Pure SVG, theme-agnostic (uses its own gradient), scales crisply at any size.
 */
export function BrandLogo({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 32 32"
      role="img"
      aria-label="ZKit"
      className={cn("size-8 shrink-0", className)}
    >
      <defs>
        <linearGradient id="zlk-tile" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#3b9bff" />
          <stop offset="1" stopColor="#1677ff" />
        </linearGradient>
      </defs>
      <rect x="0" y="0" width="32" height="32" rx="8" fill="url(#zlk-tile)" />
      {/* stylized "z": top bar, diagonal, baseline bar */}
      <path
        d="M9 10.5h14l-11 11h11"
        fill="none"
        stroke="#fff"
        strokeWidth="2.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      {/* spark dot — the "Lab" accent */}
      <circle cx="22.5" cy="9.5" r="2.2" fill="#bfe0ff" />
    </svg>
  );
}
