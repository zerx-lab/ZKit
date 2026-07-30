import { useSyncExternalStore } from "react";

// Below this width the sidebar switches to an off-canvas drawer. Matches
// Tailwind's `md` breakpoint (768px) used by the layout classes.
const QUERY = "(max-width: 767px)";

function subscribe(callback: () => void) {
  const mql = window.matchMedia(QUERY);
  mql.addEventListener("change", callback);
  return () => mql.removeEventListener("change", callback);
}

// useIsMobile reports whether the viewport is below the `md` breakpoint,
// re-rendering on media-query changes (no resize-listener debouncing needed).
export function useIsMobile(): boolean {
  return useSyncExternalStore(subscribe, () => window.matchMedia(QUERY).matches);
}
