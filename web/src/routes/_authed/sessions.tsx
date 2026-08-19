import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  listSessions,
  revokeSession,
} from "@/gen/zerx/v1/auth-AuthService_connectquery";
import { getSessionId } from "@/lib/auth";
import { useI18n } from "@/lib/i18n";

export const Route = createFileRoute("/_authed/sessions")({ component: SessionsPage });

function useInvalidate() {
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listSessions, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

function SessionsPage() {
  const { t } = useI18n();
  const [userIdInput, setUserIdInput] = useState("");
  const [userId, setUserId] = useState<bigint>(0n);
  const invalidate = useInvalidate();

  const { data, isPending } = useQuery(listSessions, { userId });
  const sessions = data?.sessions ?? [];

  const revoke = useMutation(revokeSession, {
    onSuccess: () => {
      toast.success(t("sessionPage.revokedToast"));
      void invalidate();
    },
    onError: (err) => {
      toast.error(errMsg(err, t("sessionPage.revokedToast")));
    },
  });

  function applyFilter() {
    const trimmed = userIdInput.trim();
    if (trimmed === "") {
      setUserId(0n);
      return;
    }
    try {
      setUserId(BigInt(trimmed));
    } catch {
      // ignore invalid input
    }
  }

  const currentSessionId = getSessionId();

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("sessionPage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("sessionPage.subtitle")}</p>
        </div>
      </div>

      <div className="flex items-end gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="userId-filter">{t("sessionPage.userIdLabel")}</Label>
          <Input
            id="userId-filter"
            className="w-48"
            placeholder="0"
            value={userIdInput}
            onChange={(e) => setUserIdInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") applyFilter();
            }}
          />
        </div>
        <Button variant="outline" onClick={applyFilter}>
          {t("sessionPage.apply")}
        </Button>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
        <Table>
          <TableHeader><TableRow>
            <TableHead>{t("common.id")}</TableHead>
            <TableHead>{t("sessionPage.ip")}</TableHead>
            <TableHead>{t("sessionPage.device")}</TableHead>
            <TableHead>{t("common.created")}</TableHead>
            <TableHead>{t("sessionPage.lastSeen")}</TableHead>
            <TableHead className="text-right">{t("common.actions")}</TableHead>
          </TableRow></TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : sessions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  {t("common.noData")}
                </TableCell>
              </TableRow>
            ) : (
              sessions.map((session) => {
                const isCurrent = currentSessionId !== null && session.id === currentSessionId;
                return (
                  <TableRow key={session.id}>
                    <TableCell className="font-mono text-xs">
                      <span title={session.id}>{session.id.slice(0, 8)}…</span>
                      {isCurrent && (
                        <Badge variant="secondary" className="ml-2 text-xs">
                          {t("common.current")}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell>{session.ip}</TableCell>
                    <TableCell className="max-w-xs truncate text-muted-foreground">
                      {session.userAgent}
                    </TableCell>
                    <TableCell>
                      {session.createdAt ? new Date(session.createdAt).toLocaleString() : "—"}
                    </TableCell>
                    <TableCell>
                      {session.lastSeenAt ? new Date(session.lastSeenAt).toLocaleString() : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={revoke.isPending}
                        onClick={() => revoke.mutate({ id: session.id })}
                      >
                        {t("sessionPage.revoke")}
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
