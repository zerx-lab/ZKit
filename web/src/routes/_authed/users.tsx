import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { type AnyFieldApi, useForm } from "@tanstack/react-form";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { DownloadIcon, PlusIcon, SearchIcon, UploadIcon } from "lucide-react";
import { type FormEvent, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Can } from "@/components/can";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { User } from "@/gen/zerx/v1/user_pb";
import { listRoles } from "@/gen/zerx/v1/role-RoleService_connectquery";
import {
  createUser,
  deleteUser,
  disableUserTotp,
  listUsers,
  resetPassword,
  updateUser,
} from "@/gen/zerx/v1/user-UserService_connectquery";
import { Switch } from "@/components/ui/switch";
import { firstErrorMessage } from "@/lib/form";
import { useI18n, type TranslateFn } from "@/lib/i18n";
import { authedFetch } from "@/lib/transport";

const PAGE_SIZE = 10;

export const Route = createFileRoute("/_authed/users")({ component: UsersPage });

function useInvalidateUsers() {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listUsers, cardinality: "finite" }),
    });
}

function TextField({
  field,
  label,
  type = "text",
  autoComplete,
}: {
  field: AnyFieldApi;
  label: string;
  type?: string;
  autoComplete?: string;
}) {
  const error = firstErrorMessage(field.state.meta.errors);
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={field.name}>{label}</Label>
      <Input
        id={field.name}
        type={type}
        autoComplete={autoComplete}
        value={field.state.value as string}
        onBlur={field.handleBlur}
        onChange={(e) => field.handleChange(e.target.value)}
      />
      {error && <p className="text-destructive text-sm">{error}</p>}
    </div>
  );
}

function RolesField({ field }: { field: AnyFieldApi }) {
  const { t } = useI18n();
  const error = firstErrorMessage(field.state.meta.errors);
  const { data } = useQuery(listRoles);
  const allRoles = data?.roles ?? [];
  const selected: string[] = (field.state.value as string[]) ?? [];

  const toggle = (code: string) => {
    const next = selected.includes(code)
      ? selected.filter((r) => r !== code)
      : [...selected, code];
    field.handleChange(next);
  };

  return (
    <div className="flex flex-col gap-2">
      <Label>{t("users.rolesLabel")}</Label>
      <div className="flex flex-wrap gap-3">
        {allRoles.map((r) => (
          <label key={r.code} className="flex items-center gap-1.5 cursor-pointer select-none text-sm">
            <Checkbox
              checked={selected.includes(r.code)}
              onCheckedChange={() => toggle(r.code)}
            />
            {r.name}
          </label>
        ))}
      </div>
      {error && <p className="text-destructive text-sm">{error}</p>}
    </div>
  );
}

const columnHelper = createColumnHelper<User>();

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function UsersPage() {
  const { t } = useI18n();
  const [page, setPage] = useState(1);
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");

  const { data, isPending } = useQuery(listUsers, {
    page: { page, pageSize: PAGE_SIZE },
    keyword,
  });

  const users = data?.users ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = keyword ? 1 : Math.max(1, Math.ceil(total / PAGE_SIZE));

  const { data: rolesData } = useQuery(listRoles);
  const roleNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const r of rolesData?.roles ?? []) {
      m.set(r.code, r.name);
    }
    return m;
  }, [rolesData]);

  const columns = useMemo(
    () => [
      columnHelper.accessor("id", {
        header: t("common.id"),
        cell: (info) => String(info.getValue()),
      }),
      columnHelper.accessor("email", { header: t("common.email") }),
      columnHelper.accessor("name", { header: t("common.name") }),
      columnHelper.accessor("roles", {
        header: t("common.roles"),
        cell: (info) => (
          <div className="flex flex-wrap gap-1">
            {info.getValue().map((r) => (
              <Badge key={r} variant={r === "admin" ? "default" : "secondary"}>
                {roleNames.get(r) ?? r}
              </Badge>
            ))}
          </div>
        ),
      }),
      columnHelper.accessor("status", {
        header: t("common.status"),
        cell: (info) => (
          <Badge variant={info.getValue() ? "default" : "secondary"}>
            {info.getValue() ? t("common.enabled") : t("common.disabled")}
          </Badge>
        ),
      }),
      columnHelper.accessor("createdAt", {
        header: t("common.created"),
        cell: (info) => new Date(info.getValue()).toLocaleString(),
      }),
      columnHelper.display({
        id: "actions",
        header: () => <span className="sr-only">{t("common.actions")}</span>,
        cell: (info) => <UserRowActions user={info.row.original} />,
      }),
    ],
    [t, roleNames],
  );

  const table = useReactTable({ data: users, columns, getCoreRowModel: getCoreRowModel() });

  const applySearch = (e: FormEvent) => {
    e.preventDefault();
    setPage(1);
    setKeyword(keywordInput.trim());
  };

  const handleExport = async () => {
    toast.info(t("users.exportToast"));
    const params = new URLSearchParams();
    if (keyword) params.set("keyword", keyword);
    const res = await authedFetch(`/api/export/users?${params.toString()}`);
    if (!res.ok) { toast.error("Export failed"); return; }
    const blob = await res.blob();
    downloadBlob(blob, "users.xlsx");
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("users.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("users.subtitle")}</p>
        </div>
        <div className="flex items-center gap-2">
          <Can code="user:export">
            <Button variant="outline" size="sm" onClick={() => void handleExport()}>
              <DownloadIcon className="size-4" />
              {t("common.export")}
            </Button>
          </Can>
          <Can code="user:import">
            <ImportUsersDialog />
          </Can>
          <Can code="user:create">
            <CreateUserDialog />
          </Can>
        </div>
      </div>

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <div className="border-b bg-card px-4 py-3"><form className="flex items-center gap-2" onSubmit={applySearch}>
          <div className="relative w-full max-w-xs">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t("users.searchPlaceholder")}
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              className="pl-8"
            />
          </div>
          <Button type="submit" variant="secondary">
            {t("common.search")}
          </Button>
        </form></div>

        <Table>
          <TableHeader>{table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(header.column.columnDef.header, header.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}</TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {t("common.loading")}
                </TableCell>
              </TableRow>
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                  {t("users.noUsers")}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">{t("users.total", { count: total })}</p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t("users.previous")}
          </Button>
          <span className="text-sm tabular-nums">{t("users.pageOf", { page, pages: pageCount })}</span>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= pageCount}
            onClick={() => setPage((p) => p + 1)}
          >
            {t("users.next")}
          </Button>
        </div></div>
      </Card>
    </div>
  );
}

function createUserSchema(t: TranslateFn) {
  return z.object({
    email: z.email(t("validation.email")),
    name: z.string().min(1, t("validation.nameRequired")),
    password: z.string().min(8, t("validation.passwordMin")),
    roles: z.array(z.string()).min(1, t("validation.rolesRequired")),
    nickname: z.string(),
    phone: z.string(),
  });
}

function CreateUserDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateUsers();
  const mutation = useMutation(createUser);

  const form = useForm({
    defaultValues: {
      email: "",
      name: "",
      password: "",
      roles: ["user"] as string[],
      nickname: "",
      phone: "",
    },
    validators: { onChange: createUserSchema(t) },
    onSubmit: async ({ value }) => {
      try {
        await mutation.mutateAsync(value);
        toast.success(t("users.createdToast"));
        await invalidate();
        form.reset();
        setOpen(false);
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>
          <PlusIcon className="size-4" />
          {t("users.add")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.addTitle")}</DialogTitle>
          <DialogDescription>{t("users.addDesc")}</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="email">
            {(field) => <TextField field={field} label={t("common.email")} type="email" />}
          </form.Field>
          <form.Field name="name">
            {(field) => <TextField field={field} label={t("common.name")} />}
          </form.Field>
          <form.Field name="nickname">
            {(field) => <TextField field={field} label={t("common.nickname")} />}
          </form.Field>
          <form.Field name="phone">
            {(field) => <TextField field={field} label={t("common.phone")} />}
          </form.Field>
          <form.Field name="password">
            {(field) => (
              <TextField
                field={field}
                label={t("common.password")}
                type="password"
                autoComplete="new-password"
              />
            )}
          </form.Field>
          <form.Field name="roles">{(field) => <RolesField field={field} />}</form.Field>
          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {t("common.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function UserRowActions({ user }: { user: User }) {
  return (
    <div className="flex justify-end gap-1 flex-wrap">
      <Can code="user:update">
        <EditUserDialog user={user} />
      </Can>
      <Can code="user:reset">
        <ResetPasswordDialog user={user} />
      </Can>
      {user.totpEnabled && (
        <Can code="user:update">
          <DisableTotpButton user={user} />
        </Can>
      )}
      <Can code="user:delete">
        <DeleteUserDialog user={user} />
      </Can>
    </div>
  );
}

function DisableTotpButton({ user }: { user: User }) {
  const { t } = useI18n();
  const invalidate = useInvalidateUsers();
  const mutation = useMutation(disableUserTotp);

  const handleDisable = async () => {
    try {
      await mutation.mutateAsync({ id: user.id });
      toast.success(t("users.disableTotpToast"));
      await invalidate();
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={mutation.isPending}
      onClick={() => void handleDisable()}
    >
      {t("users.disableTotp")}
    </Button>
  );
}

function EditUserDialog({ user }: { user: User }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateUsers();
  const mutation = useMutation(updateUser);

  const form = useForm({
    defaultValues: {
      name: user.name,
      roles: user.roles,
      nickname: user.nickname,
      phone: user.phone,
      status: user.status,
    },
    validators: {
      onChange: z.object({
        name: z.string().min(1, t("validation.nameRequired")),
        roles: z.array(z.string()).min(1, t("validation.rolesRequired")),
        nickname: z.string(),
        phone: z.string(),
        status: z.boolean(),
      }),
    },
    onSubmit: async ({ value }) => {
      try {
        await mutation.mutateAsync({
          id: user.id,
          name: value.name,
          roles: value.roles,
          nickname: value.nickname,
          phone: value.phone,
          status: value.status,
        });
        toast.success(t("users.updatedToast"));
        await invalidate();
        setOpen(false);
      } catch (err) {
        toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
      }
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          {t("common.edit")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.editTitle")}</DialogTitle>
          <DialogDescription>{user.email}</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.Field name="name">
            {(field) => <TextField field={field} label={t("common.name")} />}
          </form.Field>
          <form.Field name="nickname">
            {(field) => <TextField field={field} label={t("common.nickname")} />}
          </form.Field>
          <form.Field name="phone">
            {(field) => <TextField field={field} label={t("common.phone")} />}
          </form.Field>
          <form.Field name="roles">{(field) => <RolesField field={field} />}</form.Field>
          <form.Field name="status">
            {(field) => (
              <div className="flex items-center justify-between">
                <Label htmlFor={field.name}>{t("common.status")}</Label>
                <Switch
                  id={field.name}
                  checked={field.state.value as boolean}
                  onCheckedChange={(v) => field.handleChange(v)}
                />
              </div>
            )}
          </form.Field>
          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function DeleteUserDialog({ user }: { user: User }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidateUsers();
  const mutation = useMutation(deleteUser);

  const handleDelete = async () => {
    try {
      await mutation.mutateAsync({ id: user.id });
      toast.success(t("users.deletedToast"));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive">
          {t("common.delete")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.deleteTitle")}</DialogTitle>
          <DialogDescription>{t("users.deleteDesc", { email: user.email })}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={mutation.isPending} onClick={() => void handleDelete()}>
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ResetPasswordDialog({ user }: { user: User }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const mutation = useMutation(resetPassword);

  const handleReset = async () => {
    try {
      await mutation.mutateAsync({ id: user.id, password });
      toast.success(t("users.resetToast"));
      setPassword("");
      setOpen(false);
    } catch (err) {
      toast.error(err instanceof ConnectError ? err.message : t("register.failed"));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm">
          {t("users.reset")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.resetTitle")}</DialogTitle>
          <DialogDescription>{t("users.resetDesc", { email: user.email })}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor="newpw">{t("users.newPassword")}</Label>
          <Input
            id="newpw"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button disabled={mutation.isPending || password.length < 8} onClick={() => void handleReset()}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface ImportResult {
  created: number;
  failed: { row: number; reason: string }[];
}

function ImportUsersDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [loading, setLoading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const invalidate = useInvalidateUsers();

  const handleImport = async () => {
    const file = fileRef.current?.files?.[0];
    if (!file) return;
    setLoading(true);
    try {
      const fd = new FormData();
      fd.append("file", file);
      const res = await authedFetch("/api/import/users", { method: "POST", body: fd });
      const json = (await res.json()) as ImportResult;
      setResult(json);
      await invalidate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("register.failed"));
    } finally {
      setLoading(false);
    }
  };

  const handleTemplateDownload = async () => {
    const res = await authedFetch("/api/import/users/template");
    if (!res.ok) return;
    const blob = await res.blob();
    downloadBlob(blob, "users-template.xlsx");
  };

  return (
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) { setResult(null); } }}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <UploadIcon className="size-4" />
          {t("common.import")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.importTitle")}</DialogTitle>
          <DialogDescription>{t("users.importDesc")}</DialogDescription>
        </DialogHeader>

        {result ? (
          <div className="flex flex-col gap-3">
            <p className="text-sm">{t("users.importSuccess", { created: result.created })}</p>
            {result.failed.length > 0 && (
              <div>
                <p className="text-sm text-destructive">{t("users.importFailures", { count: result.failed.length })}</p>
                <ul className="mt-1 max-h-40 overflow-auto text-xs text-muted-foreground space-y-0.5">
                  {result.failed.map((f, i) => (
                    <li key={i}>Row {f.row}: {f.reason}</li>
                  ))}
                </ul>
              </div>
            )}
            <DialogFooter>
              <Button onClick={() => { setResult(null); setOpen(false); }}>{t("common.confirm")}</Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="import-file">{t("users.importSelect")}</Label>
              <Input id="import-file" type="file" accept=".xlsx" ref={fileRef} />
            </div>
            <button
              type="button"
              onClick={() => void handleTemplateDownload()}
              className="text-left text-xs text-primary hover:underline"
            >
              {t("users.importTemplate")}
            </button>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>{t("common.cancel")}</Button>
              <Button disabled={loading} onClick={() => void handleImport()}>
                {loading ? t("filePage.uploading") : t("users.importBtn")}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
