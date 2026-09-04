import { Code, ConnectError } from "@connectrpc/connect";
import { Link, useRouter, type ErrorComponentProps } from "@tanstack/react-router";

import { Button } from "@/components/ui/button";
import { useI18n, type TranslateFn } from "@/lib/i18n";

// describe maps a thrown error to a status label and user-facing message.
// Backend internal errors are already redacted by the server interceptor; we
// still replace them with a generic message so users never see raw text.
function describe(error: Error, t: TranslateFn): { status: string; message: string } {
  if (error instanceof ConnectError) {
    switch (error.code) {
      case Code.PermissionDenied:
        return { status: "403", message: t("errorPage.forbidden") };
      case Code.Internal:
      case Code.Unavailable:
        return { status: "500", message: t("errorPage.internal") };
      default:
        return { status: String(error.code), message: error.rawMessage };
    }
  }
  return { status: t("errorPage.title"), message: error.message || t("errorPage.internal") };
}

// ErrorView is the router-wide error boundary fallback. Retry resets the
// boundary and re-runs loaders for the current location.
export function ErrorView({ error, reset }: ErrorComponentProps) {
  const { t } = useI18n();
  const router = useRouter();
  const { status, message } = describe(error, t);

  const retry = () => {
    reset();
    void router.invalidate();
  };

  return (
    <div className="flex min-h-[60svh] flex-1 flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="text-6xl font-semibold tracking-tight text-muted-foreground/60">{status}</p>
      <div className="space-y-1">
        <h1 className="text-lg font-medium text-foreground">{t("errorPage.title")}</h1>
        <p className="max-w-md text-sm text-muted-foreground break-words">{message}</p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline" onClick={retry}>
          {t("errorPage.retry")}
        </Button>
        <Button asChild>
          <Link to="/">{t("errorPage.home")}</Link>
        </Button>
      </div>
    </div>
  );
}
