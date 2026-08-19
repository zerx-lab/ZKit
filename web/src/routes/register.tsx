import { ConnectError } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { toast } from "sonner";
import { z } from "zod";

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
import { register } from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { getSiteSettings } from "@/gen/zerx/v1/site-SiteSettingsService_connectquery";
import { auth } from "@/lib/auth";
import { firstErrorMessage } from "@/lib/form";
import { queryClient } from "@/lib/query-client";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/register")({
  component: RegisterPage,
});

function RegisterPage() {
  const { t } = useI18n();
  const router = useRouter();
  const registerMutation = useMutation(register);
  const siteQuery = useQuery(getSiteSettings, {});
  const registerOpen = siteQuery.data?.registerOpen ?? false;

  const nameSchema = z.string().min(1, t("validation.nameRequired"));
  const emailSchema = z.email(t("validation.email"));
  const passwordSchema = z.string().min(8, t("validation.passwordMin"));

  const form = useForm({
    defaultValues: { email: "", name: "", password: "" },
    onSubmit: async ({ value }) => {
      try {
        const res = await registerMutation.mutateAsync(value);
        auth.setTokens(res.accessToken, res.refreshToken, res.sessionId);
        queryClient.clear();
        router.history.push("/dashboard");
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <div className="relative flex min-h-svh items-center justify-center bg-background p-4">
      <div className="absolute right-4 top-4 flex items-center gap-1">
        <ThemeToggle />
        <LanguageSwitcher />
      </div>

      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="mb-1 flex items-center gap-2.5">
            <div className="size-8 rounded-md bg-primary" />
            <span className="text-lg font-semibold">{t("app.name")}</span>
          </div>
          <CardTitle className="text-xl">
            {registerOpen || siteQuery.isPending ? t("register.title") : t("register.closed")}
          </CardTitle>
          <CardDescription>
            {registerOpen || siteQuery.isPending ? t("register.subtitle") : t("register.closedHint")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {!siteQuery.isPending && !registerOpen ? (
            <p className="text-center text-sm text-muted-foreground">
              <Link to="/login" className="font-medium text-primary hover:underline">
                {t("register.loginLink")}
              </Link>
            </p>
          ) : (
          <>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              void form.handleSubmit();
            }}
          >
            <form.Field name="name" validators={{ onChange: nameSchema }}>
              {(field) => {
                const error = firstErrorMessage(field.state.meta.errors);
                return (
                  <div className="flex flex-col gap-2">
                    <Label htmlFor={field.name}>{t("common.name")}</Label>
                    <Input
                      id={field.name}
                      autoComplete="name"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                    {error && <p className="text-destructive text-sm">{error}</p>}
                  </div>
                );
              }}
            </form.Field>

            <form.Field name="email" validators={{ onChange: emailSchema }}>
              {(field) => {
                const error = firstErrorMessage(field.state.meta.errors);
                return (
                  <div className="flex flex-col gap-2">
                    <Label htmlFor={field.name}>{t("common.email")}</Label>
                    <Input
                      id={field.name}
                      type="email"
                      autoComplete="email"
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
                    <Label htmlFor={field.name}>{t("common.password")}</Label>
                    <Input
                      id={field.name}
                      type="password"
                      autoComplete="new-password"
                      value={field.state.value}
                      onBlur={field.handleBlur}
                      onChange={(e) => field.handleChange(e.target.value)}
                    />
                    {error && <p className="text-destructive text-sm">{error}</p>}
                  </div>
                );
              }}
            </form.Field>

            <Button type="submit" className="mt-1 w-full" disabled={registerMutation.isPending}>
              {registerMutation.isPending ? t("register.submitting") : t("common.register")}
            </Button>
          </form>

          <p className="mt-4 text-center text-sm text-muted-foreground">
            {t("register.haveAccount")}{" "}
            <Link to="/login" className="font-medium text-primary hover:underline">
              {t("register.loginLink")}
            </Link>
          </p>
          </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
