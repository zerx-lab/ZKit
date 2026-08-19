import { ConnectError } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

import { BrandLogo } from "@/components/brand-logo";
import { LanguageSwitcher } from "@/components/language-switcher";
import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getCaptcha, login } from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { getSiteSettings } from "@/gen/zerx/v1/site-SiteSettingsService_connectquery";
import { auth } from "@/lib/auth";
import { firstErrorMessage } from "@/lib/form";
import { queryClient } from "@/lib/query-client";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): { redirect?: string } =>
    typeof search.redirect === "string" ? { redirect: search.redirect } : {},
  component: LoginPage,
});

function LoginPage() {
  const { t } = useI18n();
  const router = useRouter();
  const search = Route.useSearch();
  const loginMutation = useMutation(login);
  const [showCaptcha, setShowCaptcha] = useState(false);
  const [captchaCode, setCaptchaCode] = useState("");
  const [totpRequired, setTotpRequired] = useState(false);
  const [totpCode, setTotpCode] = useState("");
  // Store email/password for the second TOTP step
  const [pendingCreds, setPendingCreds] = useState<{ email: string; password: string } | null>(null);

  const captchaQuery = useQuery(getCaptcha, undefined, { enabled: showCaptcha });
  const captchaImage = captchaQuery.data?.imageBase64 ?? "";
  const captchaIdFromQuery = captchaQuery.data?.captchaId ?? "";
  const siteQuery = useQuery(getSiteSettings, {});
  const registerOpen = siteQuery.data?.registerOpen ?? false;

  const emailSchema = z.email(t("validation.email"));
  const passwordSchema = z.string().min(8, t("validation.passwordMin"));

  const doLogin = async (email: string, password: string, totp: string) => {
    const res = await loginMutation.mutateAsync({
      email,
      password,
      captchaId: captchaIdFromQuery,
      captchaCode,
      totpCode: totp,
    });
    if (res.totpRequired) {
      setPendingCreds({ email, password });
      setTotpRequired(true);
      return;
    }
    auth.setTokens(res.accessToken, res.refreshToken, res.sessionId);
    queryClient.clear();
    router.history.push(search.redirect ?? "/dashboard");
  };

  const form = useForm({
    defaultValues: { email: "", password: "" },
    onSubmit: async ({ value }) => {
      try {
        await doLogin(value.email, value.password, "");
      } catch (err) {
        setShowCaptcha(true);
        setCaptchaCode("");
        void captchaQuery.refetch();
        toast.error(err instanceof ConnectError ? err.message : t("login.failed"));
      }
    },
  });

  const handleTotpSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!pendingCreds) return;
    try {
      await doLogin(pendingCreds.email, pendingCreds.password, totpCode);
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("login.failed"));
    }
  };

  return (
    <div className="relative flex min-h-svh items-center justify-center bg-background p-4">
      <div className="absolute right-4 top-4 flex items-center gap-1">
        <ThemeToggle />
        <LanguageSwitcher />
      </div>

      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="mb-1 flex items-center gap-2.5">
            <BrandLogo />
            <span className="text-lg font-semibold">{t("app.name")}</span>
          </div>
          <CardTitle className="text-xl">{t("login.title")}</CardTitle>
          <CardDescription>{t("login.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          {totpRequired ? (
            /* TOTP step */
            <form className="flex flex-col gap-4" onSubmit={handleTotpSubmit}>
              <p className="text-sm text-muted-foreground">{t("login.totpPrompt")}</p>
              <div className="flex flex-col gap-2">
                <Label htmlFor="totp">{t("login.totpLabel")}</Label>
                <Input
                  id="totp"
                  autoFocus
                  autoComplete="one-time-code"
                  placeholder={t("login.totpPlaceholder")}
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value)}
                />
              </div>
              <Button
                type="submit"
                className="mt-1 w-full"
                disabled={loginMutation.isPending || totpCode.length < 6}
              >
                {loginMutation.isPending ? t("login.submitting") : t("common.signIn")}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setTotpRequired(false);
                  setPendingCreds(null);
                  setTotpCode("");
                }}
              >
                {t("common.cancel")}
              </Button>
            </form>
          ) : (
            /* Normal login step */
            <form
              className="flex flex-col gap-4"
              onSubmit={(e) => {
                e.preventDefault();
                void form.handleSubmit();
              }}
            >
              <form.Field name="email" validators={{ onChange: emailSchema }}>
                {(field) => {
                  const error = firstErrorMessage(field.state.meta.errors);
                  return (
                    <div className="flex flex-col gap-2">
                      <Label htmlFor={field.name}>{t("common.email")}</Label>
                      <Input
                        id={field.name}
                        type="email"
                        autoComplete="username"
                        placeholder="you@example.com"
                        value={field.state.value}
                        onBlur={field.handleBlur}
                        onChange={(e) => field.handleChange(e.target.value)}
                      />
                      {error && <p className="text-destructive text-sm">{error}</p>}
                    </div>
                  );
                }}
              </form.Field>

              <form.Field name="password" validators={{ onChange: passwordSchema }}>
                {(field) => {
                  const error = firstErrorMessage(field.state.meta.errors);
                  return (
                    <div className="flex flex-col gap-2">
                      <div className="flex items-center justify-between">
                        <Label htmlFor={field.name}>{t("common.password")}</Label>
                        <Link
                          to="/forgot-password"
                          className="text-xs text-muted-foreground hover:text-primary hover:underline"
                        >
                          {t("login.forgotPassword")}
                        </Link>
                      </div>
                      <Input
                        id={field.name}
                        type="password"
                        autoComplete="current-password"
                        value={field.state.value}
                        onBlur={field.handleBlur}
                        onChange={(e) => field.handleChange(e.target.value)}
                      />
                      {error && <p className="text-destructive text-sm">{error}</p>}
                    </div>
                  );
                }}
              </form.Field>

              {showCaptcha && (
                <div className="flex flex-col gap-2">
                  <Label htmlFor="captcha">{t("login.captchaLabel")}</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="captcha"
                      autoComplete="off"
                      placeholder={t("login.captchaPlaceholder")}
                      value={captchaCode}
                      onChange={(e) => setCaptchaCode(e.target.value)}
                    />
                    {captchaImage ? (
                      <img
                        src={captchaImage}
                        alt="captcha"
                        className="h-9 cursor-pointer rounded border"
                        onClick={() => void captchaQuery.refetch()}
                      />
                    ) : null}
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("login.captchaRefresh")}
                      onClick={() => void captchaQuery.refetch()}
                    >
                      <RefreshCwIcon className="size-4" />
                    </Button>
                  </div>
                </div>
              )}

              <Button type="submit" className="mt-1 w-full" disabled={loginMutation.isPending}>
                {loginMutation.isPending ? t("login.submitting") : t("common.signIn")}
              </Button>
            </form>
          )}

          {registerOpen ? (
            <p className="mt-4 text-center text-sm text-muted-foreground">
              {t("login.noAccount")}{" "}
              <Link to="/register" className="font-medium text-primary hover:underline">
                {t("login.registerLink")}
              </Link>
            </p>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
