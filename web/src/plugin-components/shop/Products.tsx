import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  createProduct,
  deleteProduct,
  listProducts,
  updateProduct,
} from "@/gen/zerx/v1/shop-ShopProductService_connectquery";
import type { ShopProduct } from "@/gen/zerx/v1/shop_pb";
import { usePermissions } from "@/lib/permissions";
import { useI18n } from "@/lib/i18n";

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

// Products is the example grouped plugin's product sub-page, loaded dynamically
// by the /p/$ splat route via import.meta.glob keyed on the menu's component
// field ("shop/Products").
export default function Products() {
  const { t } = useI18n();
  const { can } = usePermissions();
  const qc = useQueryClient();
  const [keyword, setKeyword] = useState("");
  const [editing, setEditing] = useState<ShopProduct | null>(null);
  const [creating, setCreating] = useState(false);

  const { data, isPending } = useQuery(listProducts, {
    page: { page: 1, pageSize: 50 },
    keyword,
  });
  const products = data?.products ?? [];

  const invalidate = () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listProducts, cardinality: "finite" }),
    });

  const remove = useMutation(deleteProduct, {
    onSuccess: () => {
      void invalidate();
      toast.success(t("plg.shop.deletedToast"));
    },
    onError: (err) => toast.error(errMsg(err, t("common.error"))),
  });

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold">{t("plg.shop.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("plg.shop.subtitle")}</p>
        </div>
        {can("plg_shop_product:create") && (
          <Button onClick={() => setCreating(true)}>{t("plg.shop.add")}</Button>
        )}
      </div>

      <div className="flex gap-2">
        <Input
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t("plg.shop.name")}
          className="max-w-xs"
        />
      </div>

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t("plg.shop.name")}</TableHead>
              <TableHead>{t("plg.shop.price")}</TableHead>
              <TableHead>{t("plg.shop.description")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={5}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ) : products.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground">
                  {t("plg.shop.empty")}
                </TableCell>
              </TableRow>
            ) : (
              products.map((p) => (
                <TableRow key={String(p.id)}>
                  <TableCell>{String(p.id)}</TableCell>
                  <TableCell>{p.name}</TableCell>
                  <TableCell>{String(p.price)}</TableCell>
                  <TableCell className="max-w-xs truncate">{p.description}</TableCell>
                  <TableCell className="text-right">
                    {can("plg_shop_product:update") && (
                      <Button variant="ghost" size="sm" onClick={() => setEditing(p)}>
                        {t("common.edit")}
                      </Button>
                    )}
                    {can("plg_shop_product:delete") && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => remove.mutate({ id: p.id })}
                      >
                        {t("common.delete")}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>

      {creating && (
        <ProductDialog
          title={t("plg.shop.add")}
          onClose={() => setCreating(false)}
          onDone={() => {
            setCreating(false);
            void invalidate();
          }}
        />
      )}
      {editing && (
        <ProductDialog
          title={t("plg.shop.edit")}
          product={editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void invalidate();
          }}
        />
      )}
    </div>
  );
}

function ProductDialog({
  title,
  product,
  onClose,
  onDone,
}: {
  title: string;
  product?: ShopProduct;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useI18n();
  const [name, setName] = useState(product?.name ?? "");
  const [price, setPrice] = useState(String(product?.price ?? 0n));
  const [description, setDescription] = useState(product?.description ?? "");

  const create = useMutation(createProduct, {
    onSuccess: () => {
      toast.success(t("plg.shop.savedToast"));
      onDone();
    },
    onError: (err) => toast.error(errMsg(err, t("plg.shop.savedToast"))),
  });
  const update = useMutation(updateProduct, {
    onSuccess: () => {
      toast.success(t("plg.shop.savedToast"));
      onDone();
    },
    onError: (err) => toast.error(errMsg(err, t("plg.shop.savedToast"))),
  });

  function submit() {
    const priceNum = /^\d+$/.test(price.trim()) ? BigInt(price.trim()) : 0n;
    if (product) {
      update.mutate({ id: product.id, name, price: priceNum, description });
    } else {
      create.mutate({ name, price: priceNum, description });
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label>{t("plg.shop.name")}</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-2">
            <Label>{t("plg.shop.price")}</Label>
            <Input
              type="number"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>{t("plg.shop.description")}</Label>
            <Input value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} disabled={create.isPending || update.isPending}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
