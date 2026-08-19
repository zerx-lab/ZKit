import { ConnectError } from "@connectrpc/connect";
import { createConnectQueryKey, useMutation, useQuery } from "@connectrpc/connect-query";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
  createCategory,
  deleteCategory,
  listCategories,
} from "@/gen/zerx/v1/shop-ShopCategoryService_connectquery";
import { usePermissions } from "@/lib/permissions";
import { useI18n } from "@/lib/i18n";

function errMsg(err: unknown, fallback: string) {
  return err instanceof ConnectError ? err.message : fallback;
}

// Categories is the second sub-page of the grouped "shop" plugin, loaded by the
// /p/$ splat route from the menu component id "shop/Categories".
export default function Categories() {
  const { t } = useI18n();
  const { can } = usePermissions();
  const qc = useQueryClient();
  const [name, setName] = useState("");

  const { data, isPending } = useQuery(listCategories, { page: { page: 1, pageSize: 50 } });
  const categories = data?.categories ?? [];

  const invalidate = () =>
    qc.invalidateQueries({
      queryKey: createConnectQueryKey({ schema: listCategories, cardinality: "finite" }),
    });

  const create = useMutation(createCategory, {
    onSuccess: () => {
      setName("");
      void invalidate();
      toast.success(t("plg.shop.savedToast"));
    },
    onError: (err) => toast.error(errMsg(err, t("common.error"))),
  });
  const remove = useMutation(deleteCategory, {
    onSuccess: () => {
      void invalidate();
      toast.success(t("plg.shop.deletedToast"));
    },
    onError: (err) => toast.error(errMsg(err, t("common.error"))),
  });

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold">{t("plg.shop.categories")}</h1>
        <p className="text-sm text-muted-foreground">{t("plg.shop.categoriesSubtitle")}</p>
      </div>

      {can("plg_shop_category:create") && (
        <div className="flex gap-2">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("plg.shop.name")}
            className="max-w-xs"
          />
          <Button
            onClick={() => name.trim() && create.mutate({ name: name.trim() })}
            disabled={create.isPending}
          >
            {t("common.add")}
          </Button>
        </div>
      )}

      <Card className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t("plg.shop.name")}</TableHead>
              <TableHead className="text-right">{t("common.actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={3}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ) : categories.length === 0 ? (
              <TableRow>
                <TableCell colSpan={3} className="text-center text-muted-foreground">
                  {t("plg.shop.empty")}
                </TableCell>
              </TableRow>
            ) : (
              categories.map((c) => (
                <TableRow key={String(c.id)}>
                  <TableCell>{String(c.id)}</TableCell>
                  <TableCell>{c.name}</TableCell>
                  <TableCell className="text-right">
                    {can("plg_shop_category:delete") && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => remove.mutate({ id: c.id })}
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
    </div>
  );
}
