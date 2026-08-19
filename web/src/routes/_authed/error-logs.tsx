import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { DownloadIcon, TrashIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Can } from "@/components/can";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cleanLogs, listErrorLogs } from "@/gen/zerx/v1/log-LogService_connectquery";
import { LogType } from "@/gen/zerx/v1/log_pb";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/error-logs")({ component: ErrorLogsPage });

const PAGE_SIZE = 10;

function useInvalidate() {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listErrorLogs, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function toStartRFC3339(date: string) {
  if (!date) return "";
  return new Date(date + "T00:00:00").toISOString();
}

function toEndRFC3339(date: string) {
  if (!date) return "";
  return new Date(date + "T23:59:59").toISOString();
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function ErrorLogsPage() {
  const { t } = useI18n();
  const { roles } = usePermissions();
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [methodFilter, setMethodFilter] = useState("");
  const [startDate, setStartDate] = useState("");
  const [endDate, setEndDate] = useState("");

  const queryInput = {
    page: { page, pageSize: PAGE_SIZE },
    keyword,
    status: statusFilter,
    method: methodFilter,
    startAt: toStartRFC3339(startDate),
    endAt: toEndRFC3339(endDate),
  };

  const { data, isPending } = useQuery(listErrorLogs, queryInput);

  const logs = data?.logs ?? [];
  const total = Number(data?.total ?? 0n);
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const handleSearch = () => {
    setKeyword(searchInput);
    setPage(1);
  };

  const handleExport = async () => {
    toast.info(t("logPage.exportToast"));
    const params = new URLSearchParams();
    if (keyword) params.set("keyword", keyword);
    if (statusFilter) params.set("status", statusFilter);
    if (methodFilter) params.set("method", methodFilter);
    if (startDate) params.set("start_at", toStartRFC3339(startDate));
    if (endDate) params.set("end_at", toEndRFC3339(endDate));
    const res = await authedFetch(`/api/export/error-logs?${params.toString()}`);
    if (!res.ok) { toast.error("Export failed"); return; }
    const blob = await res.blob();
    downloadBlob(blob, "error-logs.xlsx");
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("logPage.errorTitle")}</h1>
          <p className="text-sm text-muted-foreground">{t("logPage.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Can code="error-log:export">
            <Button variant="outline" size="sm" onClick={() => void handleExport()}>
              <DownloadIcon className="size-4" />
              {t("common.export")}
            </Button>
          </Can>
          {roles.includes("admin") && <CleanLogsDialog />}
        </div>
      </div>

      {/* Filters */}
      <div className="flex shrink-0 flex-wrap gap-2 rounded-lg border bg-card p-3"><div className="flex items-center gap-2">
        <Input
          placeholder={t("logPage.searchPlaceholder")}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          className="w-56"
        />
        <Button variant="outline" size="sm" onClick={handleSearch}>
          {t("common.search")}
        </Button>
      </div>
      <Select
        value={statusFilter}
        onValueChange={(v) => { setStatusFilter(v === "_all" ? "" : v); setPage(1); }}
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder={t("logPage.allStatuses")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="_all">{t("logPage.allStatuses")}</SelectItem>
          <SelectItem value="permission_denied">permission_denied</SelectItem>
          <SelectItem value="unauthenticated">unauthenticated</SelectItem>
          <SelectItem value="invalid_argument">invalid_argument</SelectItem>
          <SelectItem value="not_found">not_found</SelectItem>
          <SelectItem value="internal">internal</SelectItem>
        </SelectContent>
      </Select>
      <Input
        placeholder={t("logPage.methodPlaceholder")}
        value={methodFilter}
        onChange={(e) => { setMethodFilter(e.target.value); setPage(1); }}
        className="w-32"
      />
      <div className="flex items-center gap-1">
        <Label className="text-sm text-muted-foreground whitespace-nowrap">{t("logPage.filterStartDate")}</Label>
        <Input
          type="date"
          value={startDate}
          onChange={(e) => { setStartDate(e.target.value); setPage(1); }}
          className="w-36"
        />
      </div>
      <div className="flex items-center gap-1">
        <Label className="text-sm text-muted-foreground whitespace-nowrap">{t("logPage.filterEndDate")}</Label>
        <Input
          type="date"
          value={endDate}
          onChange={(e) => { setEndDate(e.target.value); setPage(1); }}
          className="w-36"
        />
      </div></div>

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
        <Table>
          <TableHeader><TableRow>
            <TableHead className="w-20">{t("common.id")}</TableHead>
            <TableHead>{t("logPage.procedure")}</TableHead>
            <TableHead className="w-24">{t("logPage.status")}</TableHead>
            <TableHead className="w-32">{t("logPage.ip")}</TableHead>
            <TableHead>{t("logPage.error")}</TableHead>
            <TableHead className="w-24">User ID</TableHead>
            <TableHead className="w-40">{t("common.created")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log) => (
                <TableRow key={String(log.id)}>
                  <TableCell className="font-mono text-xs">{String(log.id)}</TableCell>
                  <TableCell className="max-w-xs truncate font-mono text-xs">{log.procedure}</TableCell>
                  <TableCell>
                    <span className="rounded-sm bg-destructive/10 px-1.5 py-0.5 text-xs font-medium text-destructive">
                      {log.status}
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{log.ip}</TableCell>
                  <TableCell className="max-w-xs">
                    {log.error ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="block max-w-xs cursor-default truncate text-sm text-destructive">
                            {log.error}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className="max-w-sm break-words">
                          <p>{log.error}</p>
                          {log.stack && (
                            <pre className="mt-1 whitespace-pre-wrap text-xs opacity-80">
                              {log.stack.length > 600 ? log.stack.slice(0, 600) + "…" : log.stack}
                            </pre>
                          )}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{String(log.userId)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {log.createdAt ? new Date(log.createdAt).toLocaleString() : "—"}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>{t("common.total", { count: total })}</span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t("common.previous")}
          </Button>
          <span>{t("common.pageOf", { page, pages: totalPages })}</span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("common.next")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function CleanLogsDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [days, setDays] = useState(30);
  const invalidate = useInvalidate();
  const mut = useMutation(cleanLogs);

  const handleClean = async () => {
    try {
      const res = await mut.mutateAsync({ type: LogType.OPERATION, days });
      toast.success(t("logPage.cleanedToast", { count: Number(res.removed) }));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline">
          <TrashIcon className="size-4" />
          {t("logPage.clean")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("logPage.clean")}</DialogTitle>
          <DialogDescription>{t("logPage.subtitle")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor="days-error">{t("logPage.cleanDays")}</Label>
          <Input
            id="days-error"
            type="number"
            min={1}
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={mut.isPending} onClick={() => void handleClean()}>
            {t("common.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
