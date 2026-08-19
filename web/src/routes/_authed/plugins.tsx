import { Code, ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { ChevronDownIcon, ChevronRightIcon, Trash2Icon, UploadIcon } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { PluginInfo } from "@/gen/zerx/v1/plugin_pb";
import {
  installPlugin,
  listPlugins,
  setPluginEnabled,
  uninstallPlugin,
} from "@/gen/zerx/v1/plugin-PluginService_connectquery";
import { getUserMenus } from "@/gen/zerx/v1/menu-MenuService_connectquery";
import { usePermissions } from "@/lib/permissions";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/plugins")({ component: PluginsPage });

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

// isRestartDrop reports whether an error is a transport-level connection drop
// (server restarting after a source change) rather than a real RPC failure.
function isRestartDrop(err: unknown): boolean {
  if (err instanceof ConnectError) {
    return err.code === Code.Unavailable || err.code === Code.Aborted || err.code === Code.Canceled;
  }
  // Non-ConnectError (e.g. fetch TypeError on a reset connection).
  return true;
}

function PluginsPage() {
  const { t } = useI18n();
  const { can } = usePermissions();
  const qc = useQueryClient();
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [uninstallTarget, setUninstallTarget] = useState<string | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const { data, isPending } = useQuery(listPlugins, {});
  const plugins = data?.plugins ?? [];
  const uploadAllowed = data?.uploadAllowed ?? false;

  // pendingMsg localizes the server's locale-independent code ("dev"/"prod").
  const pendingMsg = (code: string) =>
    code === "prod" ? t("pluginPage.pendingProd") : t("pluginPage.pendingDev");

  const invalidateList = () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listPlugins, cardinality: "finite" }),
    });

  const toggle = useMutation(setPluginEnabled, {
    onSuccess: () => {
      // Refresh the plugin list and the sidebar menus (a disabled plugin's
      // menus are filtered out of GetUserMenus).
      void invalidateList();
      void qc.invalidateQueries({
        queryKey: createConnectQueryKey({ schema: getUserMenus, cardinality: "finite" }),
      });
      toast.success(t("pluginPage.toggled"));
    },
    onError: (err) => toast.error(errMsg(err, t("common.error"))),
  });

  // A successful install/uninstall writes source then (dev) triggers an air
  // rebuild that can restart the server before the response flushes, dropping
  // the connection. Treat a transport-level drop as "submitted, restarting"
  // rather than a hard failure (the file ops already succeeded server-side).
  const onWriteError = (err: unknown) => {
    void invalidateList();
    if (isRestartDrop(err)) {
      toast.info(t("pluginPage.restarting"), { duration: 10000 });
    } else {
      toast.error(errMsg(err, t("common.error")));
    }
  };

  const install = useMutation(installPlugin, {
    onSuccess: (res) => {
      void invalidateList();
      toast.success(t("pluginPage.installed", { name: res.name }) + " " + pendingMsg(res.message), {
        duration: 10000,
      });
    },
    onError: onWriteError,
  });

  const uninstall = useMutation(uninstallPlugin, {
    onSuccess: (res) => {
      void invalidateList();
      toast.success(t("pluginPage.uninstalled") + " " + pendingMsg(res.message), { duration: 10000 });
    },
    onError: onWriteError,
  });

  const canManage = can("plugin:manage");
  // Install/uninstall additionally require the server upload gate (off in prod by
  // default); enable/disable stays available regardless.
  const canInstall = canManage && uploadAllowed;

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-selecting the same file
    if (!file) return;
    const bytes = new Uint8Array(await file.arrayBuffer());
    install.mutate({ package: bytes });
  }

  function confirmUninstall(purgeData: boolean) {
    if (!uninstallTarget) return;
    uninstall.mutate({ name: uninstallTarget, purgeData });
    setUninstallTarget(null);
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">{t("pluginPage.title")}</h1>
        {canInstall && (
          <div>
            <input
              ref={fileRef}
              type="file"
              accept=".zip"
              className="hidden"
              onChange={onFile}
            />
            <Button onClick={() => fileRef.current?.click()} disabled={install.isPending}>
              <UploadIcon className="size-4" />
              {t("pluginPage.upload")}
            </Button>
          </div>
        )}
      </div>

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-8" />
              <TableHead>{t("pluginPage.name")}</TableHead>
              <TableHead>{t("pluginPage.services")}</TableHead>
              <TableHead className="text-center">{t("pluginPage.tables")}</TableHead>
              <TableHead className="text-center">{t("pluginPage.menus")}</TableHead>
              <TableHead className="text-center">{t("pluginPage.jobs")}</TableHead>
              <TableHead className="text-center">{t("pluginPage.publicPages")}</TableHead>
              <TableHead className="text-right">{t("pluginPage.enabled")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={8}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ) : plugins.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="text-center text-muted-foreground">
                  {t("pluginPage.empty")}
                </TableCell>
              </TableRow>
            ) : (
              plugins.map((p) => (
                <PluginRow
                  key={p.name}
                  plugin={p}
                  open={!!expanded[p.name]}
                  canManage={canManage}
                  canInstall={canInstall}
                  toggling={toggle.isPending}
                  uninstalling={uninstall.isPending}
                  onToggleOpen={() =>
                    setExpanded((e) => ({ ...e, [p.name]: !e[p.name] }))
                  }
                  onToggleEnabled={(enabled) => toggle.mutate({ name: p.name, enabled })}
                  onUninstall={() => setUninstallTarget(p.name)}
                />
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={uninstallTarget !== null} onOpenChange={(o) => !o && setUninstallTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("pluginPage.uninstallTitle", { name: uninstallTarget ?? "" })}
            </DialogTitle>
            <DialogDescription>{t("pluginPage.uninstallDesc")}</DialogDescription>
          </DialogHeader>
          <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {t("pluginPage.uninstallPurgeWarn")}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setUninstallTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button variant="secondary" onClick={() => confirmUninstall(false)}>
              {t("pluginPage.uninstallKeep")}
            </Button>
            <Button variant="destructive" onClick={() => confirmUninstall(true)}>
              {t("pluginPage.uninstallPurge")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function PluginRow({
  plugin: p,
  open,
  canManage,
  canInstall,
  toggling,
  uninstalling,
  onToggleOpen,
  onToggleEnabled,
  onUninstall,
}: {
  plugin: PluginInfo;
  open: boolean;
  canManage: boolean;
  canInstall: boolean;
  toggling: boolean;
  uninstalling: boolean;
  onToggleOpen: () => void;
  onToggleEnabled: (enabled: boolean) => void;
  onUninstall: () => void;
}) {
  const { t } = useI18n();
  return (
    <>
      <TableRow className="cursor-pointer" onClick={onToggleOpen}>
        <TableCell>
          {open ? (
            <ChevronDownIcon className="size-4" />
          ) : (
            <ChevronRightIcon className="size-4" />
          )}
        </TableCell>
        <TableCell className="font-medium">{p.name}</TableCell>
        <TableCell>
          <div className="flex flex-wrap gap-1">
            {p.services.map((s) => (
              <Badge key={s} variant="secondary" className="font-mono text-xs">
                {s.replace("zerx.v1.", "")}
              </Badge>
            ))}
          </div>
        </TableCell>
        <TableCell className="text-center text-sm text-muted-foreground">
          {p.tables.length}
        </TableCell>
        <TableCell className="text-center text-sm text-muted-foreground">{p.menuCount}</TableCell>
        <TableCell className="text-center text-sm text-muted-foreground">
          {p.jobHandlers.length}
        </TableCell>
        <TableCell className="text-center text-sm text-muted-foreground">
          {p.publicPages.length}
        </TableCell>
        <TableCell className="text-right" onClick={(e) => e.stopPropagation()}>
          <div className="flex items-center justify-end gap-2">
            {p.pendingRemoval ? (
              <Badge variant="destructive">{t("pluginPage.pendingRemoval")}</Badge>
            ) : (
              <>
                <Badge variant={p.enabled ? "default" : "outline"}>
                  {p.enabled ? t("pluginPage.on") : t("pluginPage.off")}
                </Badge>
                <Switch
                  checked={p.enabled}
                  disabled={!canManage || toggling}
                  onCheckedChange={onToggleEnabled}
                />
                {canInstall && (
                  <Button
                    variant="ghost"
                    size="icon"
                    disabled={uninstalling}
                    onClick={onUninstall}
                    title={t("pluginPage.uninstall")}
                  >
                    <Trash2Icon className="size-4 text-destructive" />
                  </Button>
                )}
              </>
            )}
          </div>
        </TableCell>
      </TableRow>
      {open && (
        <TableRow className="bg-muted/30 hover:bg-muted/30">
          <TableCell />
          <TableCell colSpan={7}>
            <div className="grid gap-4 py-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <DetailList label={t("pluginPage.tables")} items={p.tables} />
              <DetailList
                label={t("pluginPage.menus")}
                items={p.menus.map((m) => (m.path ? `${m.name} (${m.path})` : `${m.name} (${t("pluginPage.group")})`))}
              />
              <DetailList label={t("pluginPage.jobs")} items={p.jobHandlers} />
              <DetailList
                label={t("pluginPage.publicPages")}
                items={p.publicPages.map((pg) => `${pg.path} -> ${pg.component}`)}
              />
            </div>
            <p className="pb-2 text-xs text-muted-foreground">{t("pluginPage.dataRetained")}</p>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function DetailList({ label, items }: { label: string; items: string[] }) {
  const { t } = useI18n();
  return (
    <div className="flex flex-col gap-1">
      <span className="font-medium">{label}</span>
      {items.length === 0 ? (
        <span className="text-muted-foreground">{t("common.noData")}</span>
      ) : (
        <ul className="flex flex-col gap-0.5">
          {items.map((it) => (
            <li key={it} className="font-mono text-xs text-muted-foreground">
              {it}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
