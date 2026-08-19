import { ConnectError } from "@connectrpc/connect";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { useForm } from "@tanstack/react-form";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

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
import { Separator } from "@/components/ui/separator";
import {
  activateTotp,
  changePassword,
  disableTotp,
  me,
  setupTotp,
  updateProfile,
} from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { firstErrorMessage } from "@/lib/form";
import { useI18n } from "@/lib/i18n";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/profile")({
  component: ProfilePage,
});

function ProfilePage() {
  const { t } = useI18n();
  const { data: meData, isPending } = useQuery(me);
  const user = meData?.user;

  return (
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-auto">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{t("profile.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("profile.subtitle")}</p>
      </div>

      {isPending || !user ? null : (
        <>
          <ProfileForm user={user} />
          <ChangePasswordForm />
          <TwoFactorSection totpEnabled={user.totpEnabled} />
        </>
      )}
    </div>
  );
}

function ProfileForm({ user }: { user: { nickname: string; avatar: string; phone: string } }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const mutation = useMutation(updateProfile);
  const [avatarUrl, setAvatarUrl] = useState(user.avatar);
  const [uploading, setUploading] = useState(false);

  const form = useForm({
    defaultValues: {
      nickname: user.nickname,
      phone: user.phone,
    },
    validators: {
      onChange: z.object({
        nickname: z.string(),
        phone: z.string(),
      }),
    },
    onSubmit: async ({ value }) => {
      try {
        await mutation.mutateAsync({
          nickname: value.nickname,
          avatar: avatarUrl,
          phone: value.phone,
        });
        await queryClient.invalidateQueries();
        toast.success(t("profile.savedToast"));
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  const handleAvatarUpload = async (file: File) => {
    setUploading(true);
    try {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("visibility", "authenticated");
      const res = await authedFetch("/api/upload", { method: "POST", body: fd });
      if (!res.ok) throw new Error(await res.text());
      const json = (await res.json()) as { url: string };
      setAvatarUrl(json.url);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.profileSection")}</CardTitle>
        <CardDescription>{t("profile.profileDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
        >
          {/* Avatar */}
          <div className="flex flex-col gap-2">
            <Label>{t("profile.avatarLabel")}</Label>
            <div className="flex items-center gap-3">
              {avatarUrl && (
                <img
                  src={avatarUrl}
                  alt="avatar"
                  className="size-12 rounded-full object-cover border"
                />
              )}
              <div className="flex items-center gap-2">
                <Input
                  value={avatarUrl}
                  onChange={(e) => setAvatarUrl(e.target.value)}
                  placeholder="https://…"
                  className="max-w-xs"
                />
                <label className="cursor-pointer">
                  <Button type="button" variant="outline" size="sm" disabled={uploading} asChild>
                    <span>{uploading ? t("filePage.uploading") : t("profile.avatarUpload")}</span>
                  </Button>
                  <input
                    type="file"
                    accept="image/*"
                    className="sr-only"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) void handleAvatarUpload(f);
                    }}
                  />
                </label>
              </div>
            </div>
          </div>

          <form.Field name="nickname">
            {(field) => {
              const error = firstErrorMessage(field.state.meta.errors);
              return (
                <div className="flex flex-col gap-2">
                  <Label htmlFor={field.name}>{t("common.nickname")}</Label>
                  <Input
                    id={field.name}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                  />
                  {error && <p className="text-destructive text-sm">{error}</p>}
                </div>
              );
            }}
          </form.Field>

          <form.Field name="phone">
            {(field) => {
              const error = firstErrorMessage(field.state.meta.errors);
              return (
                <div className="flex flex-col gap-2">
                  <Label htmlFor={field.name}>{t("common.phone")}</Label>
                  <Input
                    id={field.name}
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                  />
                  {error && <p className="text-destructive text-sm">{error}</p>}
                </div>
              );
            }}
          </form.Field>

          <div className="flex justify-end">
            <Button type="submit" disabled={mutation.isPending}>
              {t("common.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function ChangePasswordForm() {
  const { t } = useI18n();
  const mutation = useMutation(changePassword);
  const [confirm, setConfirm] = useState("");

  const form = useForm({
    defaultValues: { oldPassword: "", newPassword: "" },
    validators: {
      onChange: z.object({
        oldPassword: z.string().min(1, t("validation.required")),
        newPassword: z.string().min(8, t("validation.passwordMin")),
      }),
    },
    onSubmit: async ({ value }) => {
      if (value.newPassword !== confirm) {
        toast.error(t("profile.passwordMismatch"));
        return;
      }
      try {
        await mutation.mutateAsync({
          oldPassword: value.oldPassword,
          newPassword: value.newPassword,
        });
        toast.success(t("profile.passwordChanged"));
        form.reset();
        setConfirm("");
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.passwordSection")}</CardTitle>
        <CardDescription>{t("profile.passwordDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="oldPassword">
            {(field) => {
              const error = firstErrorMessage(field.state.meta.errors);
              return (
                <div className="flex flex-col gap-2">
                  <Label htmlFor={field.name}>{t("profile.oldPassword")}</Label>
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

          <form.Field name="newPassword">
            {(field) => {
              const error = firstErrorMessage(field.state.meta.errors);
              return (
                <div className="flex flex-col gap-2">
                  <Label htmlFor={field.name}>{t("profile.newPassword")}</Label>
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

          <div className="flex flex-col gap-2">
            <Label htmlFor="confirm-pw">{t("profile.confirmPassword")}</Label>
            <Input
              id="confirm-pw"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
          </div>

          <div className="flex justify-end">
            <Button type="submit" disabled={mutation.isPending}>
              {t("common.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

type TotpStep = "idle" | "setup" | "activated";

function TwoFactorSection({ totpEnabled }: { totpEnabled: boolean }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const setupMutation = useMutation(setupTotp);
  const activateMutation = useMutation(activateTotp);
  const disableMutation = useMutation(disableTotp);

  const [step, setStep] = useState<TotpStep>("idle");
  const [qrBase64, setQrBase64] = useState("");
  const [secret, setSecret] = useState("");
  const [code, setCode] = useState("");
  const [disableCode, setDisableCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [showDisable, setShowDisable] = useState(false);

  const handleSetup = async () => {
    try {
      const res = await setupMutation.mutateAsync({});
      setQrBase64(res.qrImageBase64);
      setSecret(res.secret);
      setStep("setup");
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  const handleActivate = async () => {
    try {
      const res = await activateMutation.mutateAsync({ code });
      setRecoveryCodes(res.recoveryCodes);
      setStep("activated");
      await queryClient.invalidateQueries();
      toast.success(t("profile.activatedToast"));
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  const handleDisable = async () => {
    try {
      await disableMutation.mutateAsync({ code: disableCode });
      setShowDisable(false);
      setDisableCode("");
      await queryClient.invalidateQueries();
      toast.success(t("profile.disabledToast"));
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.twoFactorSection")}</CardTitle>
        <CardDescription>{t("profile.twoFactorDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {totpEnabled ? (
          /* Already enabled */
          <>
            <p className="text-sm font-medium text-green-600">{t("profile.twoFactorEnabled")}</p>
            {!showDisable ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setShowDisable(true)}
                className="w-fit"
              >
                {t("profile.disableBtn")}
              </Button>
            ) : (
              <div className="flex flex-col gap-3 max-w-sm">
                <Label htmlFor="disable-code">{t("profile.disableCodeLabel")}</Label>
                <Input
                  id="disable-code"
                  autoComplete="one-time-code"
                  value={disableCode}
                  onChange={(e) => setDisableCode(e.target.value)}
                  placeholder="123456"
                />
                <div className="flex gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => void handleDisable()}
                    disabled={disableMutation.isPending || !disableCode}
                  >
                    {t("common.confirm")}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => { setShowDisable(false); setDisableCode(""); }}
                  >
                    {t("common.cancel")}
                  </Button>
                </div>
              </div>
            )}
          </>
        ) : step === "idle" ? (
          /* Not enabled */
          <>
            <p className="text-sm text-muted-foreground">{t("profile.twoFactorDisabled")}</p>
            <Button size="sm" onClick={() => void handleSetup()} disabled={setupMutation.isPending} className="w-fit">
              {t("profile.enableBtn")}
            </Button>
          </>
        ) : step === "setup" ? (
          /* QR step */
          <div className="flex flex-col gap-4 max-w-sm">
            <p className="text-sm text-muted-foreground">{t("profile.setupStep1")}</p>
            {qrBase64 && (
              <img src={qrBase64} alt="TOTP QR" className="size-48 rounded border" />
            )}
            <details className="text-sm">
              <summary className="cursor-pointer text-muted-foreground">{t("profile.secretLabel")}</summary>
              <code className="mt-1 block break-all rounded bg-muted px-2 py-1 font-mono text-xs">{secret}</code>
            </details>
            <Separator />
            <p className="text-sm text-muted-foreground">{t("profile.setupStep2")}</p>
            <div className="flex items-center gap-2">
              <Input
                autoComplete="one-time-code"
                placeholder="123456"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                className="max-w-[12rem]"
              />
              <Button
                size="sm"
                onClick={() => void handleActivate()}
                disabled={activateMutation.isPending || code.length < 6}
              >
                {t("profile.activateBtn")}
              </Button>
            </div>
          </div>
        ) : (
          /* Activated – show recovery codes once */
          <div className="flex flex-col gap-3">
            <p className="font-medium">{t("profile.recoveryCodes")}</p>
            <p className="text-sm text-muted-foreground">{t("profile.recoveryCodesDesc")}</p>
            <div className="grid grid-cols-2 gap-1 rounded border bg-muted p-3 font-mono text-sm">
              {recoveryCodes.map((c) => (
                <span key={c}>{c}</span>
              ))}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
