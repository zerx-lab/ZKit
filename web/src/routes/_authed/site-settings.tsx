import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { UploadIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Can } from "@/components/can";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { listRoles } from "@/gen/zerx/v1/role-RoleService_connectquery";
import {
  getSiteSettings,
  updateSiteSettings,
} from "@/gen/zerx/v1/site-SiteSettingsService_connectquery";
import { useI18n } from "@/lib/i18n";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/site-settings")({ component: SiteSettingsPage });

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function SiteSettingsPage() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const { data, isPending } = useQuery(getSiteSettings, {});
  const { data: rolesData } = useQuery(listRoles);

  const [name, setName] = useState("");
  const [logo, setLogo] = useState("");
  const [domain, setDomain] = useState("");
  const [registerEnabled, setRegisterEnabled] = useState(true);
  const [registerDefaultRole, setRegisterDefaultRole] = useState("user");
  const [uploading, setUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (data) {
      setName(data.name);
      setLogo(data.logo);
      setDomain(data.domain);
      setRegisterEnabled(data.registerEnabled);
      setRegisterDefaultRole(data.registerDefaultRole || "user");
    }
  }, [data]);

  const mutation = useMutation(updateSiteSettings, {
    onSuccess: async () => {
      toast.success(t("sitePage.savedToast"));
      await queryClient.invalidateQueries({
        queryKey: createConnectQueryKey({ schema: getSiteSettings, cardinality: "finite" }),
      });
    },
    onError: (err) => toast.error(errMsg(err, t("register.failed"))),
  });

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    try {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("visibility", "public");
      const res = await authedFetch("/api/upload", { method: "POST", body: fd });
      if (res.ok) {
        const body = (await res.json().catch(() => null)) as { url?: string } | null;
        if (body?.url) setLogo(body.url);
      } else {
        const text = await res.text().catch(() => "");
        toast.error(text || t("register.failed"));
      }
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const handleSave = () => {
    mutation.mutate({ name, logo, domain, registerEnabled, registerDefaultRole });
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-auto">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">{t("sitePage.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("sitePage.subtitle")}</p>
      </div>

      <Card className="max-w-2xl p-6">
        <div className="flex flex-col gap-6">
          <div className="space-y-2">
            <Label htmlFor="site-name">{t("sitePage.name")}</Label>
            <Input
              id="site-name"
              value={name}
              placeholder={t("sitePage.namePlaceholder")}
              disabled={isPending}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="site-logo">{t("sitePage.logo")}</Label>
            <div className="flex items-center gap-3">
              <Input
                id="site-logo"
                value={logo}
                placeholder={t("sitePage.logoPlaceholder")}
                disabled={isPending}
                onChange={(e) => setLogo(e.target.value)}
              />
              <Button
                type="button"
                variant="outline"
                disabled={uploading}
                onClick={() => inputRef.current?.click()}
              >
                <UploadIcon className="size-4" />
                {uploading ? t("sitePage.uploading") : t("sitePage.uploadLogo")}
              </Button>
              <input
                ref={inputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => void handleUpload(e)}
              />
            </div>
            {logo ? (
              <img
                src={logo}
                alt={t("sitePage.logo")}
                className="mt-2 h-16 w-16 rounded border object-contain"
              />
            ) : null}
          </div>

          <div className="space-y-2">
            <Label htmlFor="site-domain">{t("sitePage.domain")}</Label>
            <Input
              id="site-domain"
              value={domain}
              placeholder={t("sitePage.domainPlaceholder")}
              disabled={isPending}
              onChange={(e) => setDomain(e.target.value)}
            />
          </div>

          <div className="flex items-start justify-between gap-4 rounded-lg border border-border px-4 py-3">
            <div className="space-y-1">
              <Label htmlFor="site-register">{t("sitePage.registerEnabled")}</Label>
              <p className="text-sm text-muted-foreground">{t("sitePage.registerEnabledHint")}</p>
            </div>
            <Switch
              id="site-register"
              checked={registerEnabled}
              disabled={isPending}
              onCheckedChange={setRegisterEnabled}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="site-register-role">{t("sitePage.registerDefaultRole")}</Label>
            <Select
              value={registerDefaultRole}
              onValueChange={setRegisterDefaultRole}
              disabled={isPending}
            >
              <SelectTrigger id="site-register-role" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(rolesData?.roles ?? [])
                  .filter((role) => role.code !== "admin")
                  .map((role) => (
                    <SelectItem key={role.code} value={role.code}>
                      {role.name || role.code}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
            <p className="text-sm text-muted-foreground">{t("sitePage.registerDefaultRoleHint")}</p>
          </div>

          <Can code="site:update">
            <div>
              <Button disabled={mutation.isPending || isPending} onClick={handleSave}>
                {mutation.isPending ? t("sitePage.saving") : t("sitePage.save")}
              </Button>
            </div>
          </Can>
        </div>
      </Card>
    </div>
  );
}
