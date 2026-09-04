import { Link } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";

// NotFound is the router-wide fallback for unmatched paths and thrown
// notFound() errors. Rendered inside whichever layout Outlet caught it, so
// under /_authed it appears within the shell.
export function NotFound() {
  const { t } = useI18n();

  return (
    <div className="flex min-h-[60svh] flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="text-6xl font-semibold tracking-tight text-muted-foreground/60">404</p>
      <div className="space-y-1">
        <h1 className="text-lg font-medium text-foreground">{t("notFound.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("notFound.description")}</p>
      </div>
      <Button asChild>
        <Link to="/">{t("notFound.home")}</Link>
      </Button>
    </div>
  );
}
