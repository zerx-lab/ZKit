import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { DownloadIcon, FileIcon, FolderUpIcon, LayoutGridIcon, ListIcon, SearchIcon, Trash2Icon, UploadIcon } from "lucide-react";
import type { FormEvent } from "react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import type { File as ZerxFile } from "@/gen/zerx/v1/file_pb";
import {
  deleteFile,
  listFiles,
} from "@/gen/zerx/v1/file-FileService_connectquery";
import { useI18n } from "@/lib/i18n";
import { usePermissions } from "@/lib/permissions";
import { authedFetch } from "@/lib/transport";

export const Route = createFileRoute("/_authed/files")({ component: FilesPage });

const PAGE_SIZE = 12;
const TEXT_PREVIEW_LIMIT = 1024 * 1024; // 1 MB cap for text preview

function useInvalidate() {
  const qc = useQueryClient();
  return () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listFiles, cardinality: "finite" }),
    });
}

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

// downloadFile fetches the blob through authedFetch (so protected files carry
// the Bearer token) and triggers a browser download via a temporary anchor.
async function downloadFile(file: ZerxFile): Promise<void> {
  const res = await authedFetch(file.url, { method: "GET" });
  if (!res.ok) throw new Error(await res.text().catch(() => ""));
  const blob = await res.blob();
  const objectUrl = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = objectUrl;
  a.download = file.name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(objectUrl);
}

function formatSize(bytes: bigint): string {
  const n = Number(bytes);
  if (n >= 1024 * 1024 * 1024) return (n / (1024 * 1024 * 1024)).toFixed(1) + " GB";
  if (n >= 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + " MB";
  if (n >= 1024) return (n / 1024).toFixed(1) + " KB";
  return n + " B";
}

function isImage(ct: string): boolean {
  return ct.startsWith("image/");
}

// Text-previewable if MIME says text/*, or a known structured-text subtype.
function isText(ct: string): boolean {
  if (ct.startsWith("text/")) return true;
  return [
    "application/json",
    "application/xml",
    "application/javascript",
    "application/x-yaml",
    "application/yaml",
  ].includes(ct.split(";")[0]?.trim() ?? "");
}

function canPreview(ct: string): boolean {
  return isImage(ct) || isText(ct);
}

type View = "list" | "gallery";

type Visibility = "private" | "authenticated" | "public";

function VisibilityBadge({ value, t }: { value: string; t: TranslateFn }) {
  const label =
    value === "public"
      ? t("filePage.visPublic")
      : value === "authenticated"
        ? t("filePage.visAuthenticated")
        : t("filePage.visPrivate");
  const variant =
    value === "public" ? "default" : value === "authenticated" ? "secondary" : "outline";
  return <Badge variant={variant}>{label}</Badge>;
}

function FilesPage() {
  const { t } = useI18n();
  const [page, setPage] = useState(1);
  const [keywordInput, setKeywordInput] = useState("");
  const [keyword, setKeyword] = useState("");
  const [view, setView] = useState<View>("list");
  const [preview, setPreview] = useState<ZerxFile | null>(null);
  const [uploadOpen, setUploadOpen] = useState(false);

  const { data, isPending } = useQuery(listFiles, {
    page: { page, pageSize: PAGE_SIZE },
    keyword,
  });
  const files = data?.files ?? [];
  const total = data ? Number(data.total) : 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const invalidate = useInvalidate();

  const applySearch = (e: FormEvent) => {
    e.preventDefault();
    setPage(1);
    setKeyword(keywordInput.trim());
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t("filePage.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("filePage.subtitle")}</p>
        </div>
        <Button onClick={() => setUploadOpen(true)}>
          <UploadIcon className="size-4" />
          {t("filePage.upload")}
        </Button>
      </div>

      <UploadDialog
        open={uploadOpen}
        onOpenChange={setUploadOpen}
        onUploaded={() => void invalidate()}
      />

      <Card className="flex min-h-0 flex-1 flex-col gap-0 overflow-hidden py-0">
        <div className="flex items-center justify-between gap-2 border-b bg-card px-4 py-3"><form className="flex items-center gap-2" onSubmit={applySearch}>
          <div className="relative w-full max-w-xs">
            <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t("filePage.searchPlaceholder")}
              value={keywordInput}
              onChange={(e) => setKeywordInput(e.target.value)}
              className="pl-8"
            />
          </div>
          <Button type="submit" variant="secondary">
            {t("common.search")}
          </Button>
        </form>
        <Tabs value={view} onValueChange={(v) => setView(v as View)}>
          <TabsList>
            <TabsTrigger value="list" aria-label={t("filePage.viewList")} title={t("filePage.viewList")}>
              <ListIcon className="size-4" />
            </TabsTrigger>
            <TabsTrigger value="gallery" aria-label={t("filePage.viewGallery")} title={t("filePage.viewGallery")}>
              <LayoutGridIcon className="size-4" />
            </TabsTrigger>
          </TabsList>
        </Tabs></div>

        {view === "list" ? (
          <ListView
            files={files}
            isPending={isPending}
            t={t}
            onPreview={setPreview}
          />
        ) : (
          <GalleryView
            files={files}
            isPending={isPending}
            t={t}
            onPreview={setPreview}
          />
        )}

        <div className="flex items-center justify-between gap-4 border-t bg-card px-4 py-3"><p className="text-sm text-muted-foreground">{t("common.total", { count: total })}</p>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            {t("common.previous")}
          </Button>
          <span className="text-sm tabular-nums">
            {t("common.pageOf", { page, pages: pageCount })}
          </span>
          <Button variant="outline" size="sm" disabled={page >= pageCount} onClick={() => setPage((p) => p + 1)}>
            {t("common.next")}
          </Button>
        </div></div>
      </Card>

      <PreviewDialog file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}

type TranslateFn = ReturnType<typeof useI18n>["t"];

function UploadDialog({
  open,
  onOpenChange,
  onUploaded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onUploaded: () => void;
}) {
  const { t } = useI18n();
  const { roles } = usePermissions();
  const isAdmin = roles.includes("admin");
  const [files, setFiles] = useState<File[]>([]);
  const [visibility, setVisibility] = useState<Visibility>("private");
  const [uploading, setUploading] = useState(false);
  const [done, setDone] = useState(0);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  const reset = () => {
    setFiles([]);
    setDone(0);
    if (fileInputRef.current) fileInputRef.current.value = "";
    if (folderInputRef.current) folderInputRef.current.value = "";
  };

  const addPicked = (e: React.ChangeEvent<HTMLInputElement>) => {
    const picked = e.target.files ? Array.from(e.target.files) : [];
    if (picked.length > 0) {
      setFiles((prev) => [...prev, ...picked]);
    }
    e.target.value = "";
  };

  const removeAt = (idx: number) => setFiles((prev) => prev.filter((_, i) => i !== idx));

  const handleOpenChange = (next: boolean) => {
    if (uploading) return;
    if (!next) reset();
    onOpenChange(next);
  };

  const startUpload = async () => {
    if (files.length === 0) return;
    setUploading(true);
    setDone(0);
    let ok = 0;
    for (const file of files) {
      try {
        const fd = new FormData();
        fd.append("file", file);
        fd.append("visibility", visibility);
        const res = await authedFetch("/api/upload", { method: "POST", body: fd });
        if (res.ok) ok += 1;
      } catch {
        // counted as failure below
      }
      setDone((d) => d + 1);
    }
    setUploading(false);
    const total = files.length;
    const failed = total - ok;
    if (failed === 0) {
      toast.success(t("filePage.uploadDoneToast", { count: ok }));
    } else {
      toast.error(t("filePage.uploadPartialToast", { ok, total, failed }));
    }
    onUploaded();
    reset();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("filePage.uploadTitle")}</DialogTitle>
          <DialogDescription>{t("filePage.uploadDialogDesc")}</DialogDescription>
        </DialogHeader>

        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={uploading}
              onClick={() => fileInputRef.current?.click()}
            >
              <UploadIcon className="size-4" />
              {t("filePage.chooseFiles")}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={uploading}
              onClick={() => folderInputRef.current?.click()}
            >
              <FolderUpIcon className="size-4" />
              {t("filePage.chooseFolder")}
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={addPicked}
            />
            <input
              ref={folderInputRef}
              type="file"
              className="hidden"
              // @ts-expect-error non-standard directory upload attributes
              webkitdirectory=""
              directory=""
              multiple
              onChange={addPicked}
            />
          </div>

          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">
              {files.length > 0
                ? t("filePage.selectedFiles", { count: files.length })
                : t("filePage.noFilesSelected")}
            </span>
            {files.length > 0 && !uploading ? (
              <Button type="button" variant="ghost" size="sm" onClick={reset}>
                {t("filePage.clearAll")}
              </Button>
            ) : null}
          </div>

          {files.length > 0 ? (
            <ul className="max-h-48 divide-y overflow-y-auto rounded-md border text-sm">
              {files.map((f, i) => (
                <li key={`${f.name}-${i}`} className="flex min-w-0 items-center justify-between gap-2 px-3 py-2">
                  <span className="min-w-0 flex-1 truncate" title={f.name}>
                    {f.name}
                  </span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {formatSize(BigInt(f.size))}
                  </span>
                  {!uploading ? (
                    <button
                      type="button"
                      className="shrink-0 text-muted-foreground hover:text-foreground"
                      aria-label={t("filePage.removeFile")}
                      onClick={() => removeAt(i)}
                    >
                      <Trash2Icon className="size-4" />
                    </button>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}

          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">{t("filePage.visibility")}</span>
            <Select
              value={visibility}
              onValueChange={(v) => setVisibility(v as Visibility)}
              disabled={uploading}
            >
              <SelectTrigger className="w-full" aria-label={t("filePage.visibility")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="private">{t("filePage.visPrivate")}</SelectItem>
                <SelectItem value="authenticated">{t("filePage.visAuthenticated")}</SelectItem>
                {isAdmin ? <SelectItem value="public">{t("filePage.visPublic")}</SelectItem> : null}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">{t("filePage.visibilityHint")}</p>
          </div>
        </div>

        <DialogFooter>
          <Button disabled={files.length === 0 || uploading} onClick={() => void startUpload()}>
            <UploadIcon className="size-4" />
            {uploading
              ? t("filePage.uploadProgress", { done, total: files.length })
              : t("filePage.startUpload")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ActionButtons({
  file,
  t,
  onPreview,
}: {
  file: ZerxFile;
  t: TranslateFn;
  onPreview: (f: ZerxFile) => void;
}) {
  const onDownload = async () => {
    try {
      await downloadFile(file);
    } catch (err) {
      toast.error(errMsg(err, t("filePage.downloadFailed")));
    }
  };
  return (
    <div className="flex justify-end gap-2">
      {canPreview(file.contentType) && (
        <Button variant="ghost" size="sm" onClick={() => onPreview(file)}>
          {t("filePage.preview")}
        </Button>
      )}
      <Button variant="ghost" size="sm" onClick={() => void onDownload()}>
        {t("filePage.download")}
      </Button>
      <Button variant="ghost" size="sm" asChild>
        <a href={file.url} target="_blank" rel="noopener noreferrer">
          {t("filePage.open")}
        </a>
      </Button>
      <DeleteFileDialog file={file} />
    </div>
  );
}

function ListView({
  files,
  isPending,
  t,
  onPreview,
}: {
  files: ZerxFile[];
  isPending: boolean;
  t: TranslateFn;
  onPreview: (f: ZerxFile) => void;
}) {
  return (
    <Table>
      <TableHeader><TableRow>
        <TableHead>{t("common.name")}</TableHead>
        <TableHead>{t("filePage.size")}</TableHead>
        <TableHead>{t("filePage.contentType")}</TableHead>
        <TableHead>{t("filePage.visibility")}</TableHead>
        <TableHead>{t("common.created")}</TableHead>
        <TableHead className="text-right">{t("common.actions")}</TableHead>
      </TableRow></TableHeader>
      <TableBody>
        {isPending ? (
          <TableRow>
            <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
              {t("common.loading")}
            </TableCell>
          </TableRow>
        ) : files.length === 0 ? (
          <TableRow>
            <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
              {t("common.noData")}
            </TableCell>
          </TableRow>
        ) : (
          files.map((f) => (
            <TableRow key={String(f.id)}>
              <TableCell className="max-w-xs truncate font-medium">{f.name}</TableCell>
              <TableCell className="text-muted-foreground">{formatSize(f.size)}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">{f.contentType}</TableCell>
              <TableCell><VisibilityBadge value={f.visibility} t={t} /></TableCell>
              <TableCell className="text-muted-foreground">
                {f.createdAt ? new Date(f.createdAt).toLocaleString() : "—"}
              </TableCell>
              <TableCell className="text-right">
                <ActionButtons file={f} t={t} onPreview={onPreview} />
              </TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}

function GalleryView({
  files,
  isPending,
  t,
  onPreview,
}: {
  files: ZerxFile[];
  isPending: boolean;
  t: TranslateFn;
  onPreview: (f: ZerxFile) => void;
}) {
  if (isPending) {
    return <div className="p-8 text-center text-sm text-muted-foreground">{t("common.loading")}</div>;
  }
  if (files.length === 0) {
    return <div className="min-h-0 flex-1 p-8 text-center text-sm text-muted-foreground">{t("common.noData")}</div>;
  }
  return (
    <div className="min-h-0 flex-1 overflow-auto grid grid-cols-2 gap-4 p-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {files.map((f) => {
        const previewable = canPreview(f.contentType);
        return (
          <div key={String(f.id)} className="group flex flex-col overflow-hidden rounded-lg border">
            <button
              type="button"
              disabled={!previewable}
              onClick={() => previewable && onPreview(f)}
              className="relative flex aspect-square shrink-0 items-center justify-center overflow-hidden bg-muted/40 transition-colors enabled:hover:bg-muted disabled:cursor-default"
            >
              {isImage(f.contentType) ? (
                <img
                  src={f.url}
                  alt={f.name}
                  loading="lazy"
                  className="absolute inset-0 size-full object-cover"
                />
              ) : (
                <FileIcon className="size-10 text-muted-foreground" />
              )}
            </button>
            <div className="flex flex-col gap-1 border-t px-2 py-1.5">
              <span className="min-w-0 truncate text-xs font-medium" title={f.name}>
                {f.name}
              </span>
              <div className="flex items-center justify-between gap-1">
                <VisibilityBadge value={f.visibility} t={t} />
                <div className="flex shrink-0 items-center gap-0.5">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="size-6 p-0 text-muted-foreground hover:text-foreground"
                    aria-label={t("filePage.download")}
                    title={t("filePage.download")}
                    onClick={() => void downloadFile(f).catch((err) => toast.error(errMsg(err, t("filePage.downloadFailed"))))}
                  >
                    <DownloadIcon className="size-3.5" />
                  </Button>
                  <DeleteFileDialog file={f} compact />
                </div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function PreviewDialog({ file, onClose }: { file: ZerxFile | null; onClose: () => void }) {
  const { t } = useI18n();
  const [text, setText] = useState<string | null>(null);
  const [textState, setTextState] = useState<"idle" | "loading" | "error">("idle");

  useEffect(() => {
    if (!file || !isText(file.contentType)) {
      setText(null);
      setTextState("idle");
      return;
    }
    let cancelled = false;
    setTextState("loading");
    setText(null);
    (async () => {
      try {
        // Only attach our auth token for same-origin URLs (local driver serves
        // relative paths). S3/presigned URLs are cross-origin — fetch without
        // credentials so the bearer token is never leaked to a third party.
        const sameOrigin = new URL(file.url, window.location.origin).origin === window.location.origin;
        const res = sameOrigin ? await authedFetch(file.url) : await fetch(file.url);
        if (!res.ok) throw new Error("fetch failed");
        const blob = await res.blob();
        const slice = blob.slice(0, TEXT_PREVIEW_LIMIT);
        const content = await slice.text();
        if (!cancelled) {
          setText(content);
          setTextState("idle");
        }
      } catch {
        if (!cancelled) setTextState("error");
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [file]);

  return (
    <Dialog open={file !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-hidden sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="truncate">{file?.name}</DialogTitle>
          <DialogDescription>{file ? formatSize(file.size) : ""}</DialogDescription>
        </DialogHeader>
        <div className="overflow-auto">
          {file && isImage(file.contentType) && (
            <img
              src={file.url}
              alt={t("filePage.previewImageAlt")}
              className="mx-auto max-h-[60vh] max-w-full rounded-md object-contain"
            />
          )}
          {file && isText(file.contentType) && (
            <>
              {textState === "loading" && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {t("filePage.previewTextLoading")}
                </p>
              )}
              {textState === "error" && (
                <p className="py-8 text-center text-sm text-destructive">
                  {t("filePage.previewLoadFailed")}
                </p>
              )}
              {textState === "idle" && text !== null && (
                <pre className="max-h-[60vh] overflow-auto rounded-md bg-muted p-4 font-mono text-xs whitespace-pre-wrap break-words">
                  {text}
                </pre>
              )}
            </>
          )}
          {file && !canPreview(file.contentType) && (
            <p className="py-8 text-center text-sm text-muted-foreground">
              {t("filePage.previewUnavailable")}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" asChild>
            <a href={file?.url} target="_blank" rel="noopener noreferrer">
              {t("filePage.open")}
            </a>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteFileDialog({ file, compact }: { file: ZerxFile; compact?: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const invalidate = useInvalidate();
  const mut = useMutation(deleteFile);

  const handleDelete = async () => {
    try {
      await mut.mutateAsync({ id: file.id });
      toast.success(t("filePage.deletedToast"));
      await invalidate();
      setOpen(false);
    } catch (err) {
      toast.error(errMsg(err, t("register.failed")));
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={
            compact
              ? "size-6 shrink-0 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
              : "text-destructive hover:bg-destructive/10 hover:text-destructive"
          }
        >
          {compact ? <Trash2Icon className="size-3.5" /> : t("common.delete")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("common.delete")}</DialogTitle>
          <DialogDescription>{file.name}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="destructive" disabled={mut.isPending} onClick={() => void handleDelete()}>
            {t("common.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
