// Apply the persisted theme before paint to avoid a flash. Served as an
// external file so the SPA works under a `script-src 'self'` CSP. Storage key
// must match STORAGE_KEY in src/lib/theme.tsx.
(function () {
  try {
    var t = localStorage.getItem("zerx.theme");
    if (t === "dark" || (!t && window.matchMedia("(prefers-color-scheme: dark)").matches)) {
      document.documentElement.classList.add("dark");
    }
  } catch {
    // localStorage may be unavailable (privacy mode); fall back to light.
  }
})();
