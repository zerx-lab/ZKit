package main

// Templates for scaffolded plugin files. Field loops emit a sensible default set
// when fields are supplied; otherwise a single name/description pair is used so
// the scaffold compiles and demonstrates the pattern.

const protoTmpl = `syntax = "proto3";

package zerx.v1;

import "buf/validate/validate.proto";
import "zerx/v1/common.proto";

option go_package = "{{.Module}}/gen/go/zerx/v1;zerxv1";

// {{.Pascal}}Service is the {{.Name}} plugin's CRUD service. Guarded by Casbin
// like any core service.
service {{.Pascal}}Service {
  rpc List{{.Pascal}}s(List{{.Pascal}}sRequest) returns (List{{.Pascal}}sResponse);
  rpc Get{{.Pascal}}(Get{{.Pascal}}Request) returns ({{.Pascal}});
  rpc Create{{.Pascal}}(Create{{.Pascal}}Request) returns ({{.Pascal}});
  rpc Update{{.Pascal}}(Update{{.Pascal}}Request) returns ({{.Pascal}});
  rpc Delete{{.Pascal}}(Delete{{.Pascal}}Request) returns (Delete{{.Pascal}}Response);
}

message {{.Pascal}} {
  uint64 id = 1;
  string name = 2;
{{- range $i, $f := .Fields}}
  {{$f.ProtoType}} {{$f.JSONName}} = {{add3 $i}};
{{- end}}
  string created_at = {{addFieldBase .Fields}};
}

message List{{.Pascal}}sRequest {
  zerx.v1.PageRequest page = 1;
  string keyword = 2;
}

message List{{.Pascal}}sResponse {
  repeated {{.Pascal}} items = 1;
  int64 total = 2;
}

message Get{{.Pascal}}Request {
  uint64 id = 1 [(buf.validate.field).uint64.gt = 0];
}

message Create{{.Pascal}}Request {
  string name = 1 [(buf.validate.field).string.min_len = 1];
{{- range $i, $f := .Fields}}
  {{$f.ProtoType}} {{$f.JSONName}} = {{add2 $i}};
{{- end}}
}

message Update{{.Pascal}}Request {
  uint64 id = 1 [(buf.validate.field).uint64.gt = 0];
  string name = 2 [(buf.validate.field).string.min_len = 1];
{{- range $i, $f := .Fields}}
  {{$f.ProtoType}} {{$f.JSONName}} = {{add3 $i}};
{{- end}}
}

message Delete{{.Pascal}}Request {
  uint64 id = 1 [(buf.validate.field).uint64.gt = 0];
}

message Delete{{.Pascal}}Response {}
`

const modelTmpl = `package {{.Name}}

import (
	"time"

	"gorm.io/gorm"
)

// {{.Pascal}} is the {{.Name}} plugin's row. TableName is namespaced plg_{{.Name}}_*.
type {{.Pascal}} struct {
	ID        uint64 ` + "`gorm:\"primaryKey\"`" + `
	Name      string ` + "`gorm:\"not null\"`" + `
{{- range .Fields}}
	{{.GoName}} {{.GoType}}
{{- end}}
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt ` + "`gorm:\"index\"`" + `
}

// TableName pins the namespaced table name.
func ({{.Pascal}}) TableName() string { return "plg_{{.Name}}_{{.Name}}s" }
`

const pluginTmpl = `// Package {{.Name}} is a ZKit plugin scaffolded by zkit.
package {{.Name}}

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"{{.Module}}/gen/go/zerx/v1/zerxv1connect"
	"{{.Module}}/internal/plugin"
)

const pluginName = "{{.Name}}"

// Plugin implements plugin.Plugin for the {{.Name}} module.
type Plugin struct{}

var _ plugin.Plugin = (*Plugin)(nil)

// New constructs the {{.Name}} plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the plugin identifier.
func (*Plugin) Name() string { return pluginName }

// Services returns the connectRPC services this plugin owns.
func (*Plugin) Services() []string {
	return []string{"zerx.v1.{{.Pascal}}Service"}
}

// Migrations returns the plugin's schema migrations (IDs prefixed plg_{{.Name}}_).
func (*Plugin) Migrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		{
			ID: "plg_{{.Name}}_0001_init",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&{{.Pascal}}{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(&{{.Pascal}}{})
			},
		},
	}
}

// TableNames returns the plugin's owned tables (prefixed plg_{{.Name}}_).
func (*Plugin) TableNames() []string {
	return []string{"plg_{{.Name}}_{{.Name}}s"}
}

// SeedMenus returns the plugin's navigation subtree.
func (*Plugin) SeedMenus() []plugin.MenuNode {
	return []plugin.MenuNode{
		{
			Name:      "plg_{{.Name}}_{{.Name}}s",
			Path:      "/p/{{.Name}}",
			Component: "{{.Name}}/{{.Pascal}}",
			Title:     "plg.{{.Name}}.title",
			Icon:      "CircleIcon",
			Sort:      2,
			Buttons: []plugin.MenuButton{
				{Code: "plg_{{.Name}}_{{.Name}}:create", Name: "{{.Pascal}} create"},
				{Code: "plg_{{.Name}}_{{.Name}}:update", Name: "{{.Pascal}} update"},
				{Code: "plg_{{.Name}}_{{.Name}}:delete", Name: "{{.Pascal}} delete"},
			},
		},
	}
}

// PublicProcedures returns no unauthenticated procedures.
func (*Plugin) PublicProcedures() []string { return nil }

// SelfServeProcedures returns no self-serve procedures.
func (*Plugin) SelfServeProcedures() []string { return nil }

// RegisterHandlers mounts the service via the shared reg + opts.
func (*Plugin) RegisterHandlers(reg plugin.RegFunc, deps plugin.Deps) {
	reg(zerxv1connect.New{{.Pascal}}ServiceHandler(New{{.Pascal}}Service(deps.DB), deps.Opts))
}

// JobHandlers returns no background jobs.
func (*Plugin) JobHandlers() map[string]plugin.JobHandler { return nil }

// PublicPages returns no anonymous front-end pages (back-office-only plugin).
// To serve a public page, return e.g.
//
//	{Path: "/pub/{{.Name}}", Component: "{{.Name}}/Landing", Title: "plg.{{.Name}}.title"}
func (*Plugin) PublicPages() []plugin.PublicPage { return nil }
`

const serviceTmpl = `package {{.Name}}

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"connectrpc.com/connect"
	"gorm.io/gorm"

	zerxv1 "{{.Module}}/gen/go/zerx/v1"
	"{{.Module}}/gen/go/zerx/v1/zerxv1connect"
	"{{.Module}}/internal/audit"
)

// Service implements zerxv1connect.{{.Pascal}}ServiceHandler.
type Service struct {
	db *gorm.DB
}

var _ zerxv1connect.{{.Pascal}}ServiceHandler = (*Service)(nil)

// New{{.Pascal}}Service constructs the handler.
func New{{.Pascal}}Service(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) List{{.Pascal}}s(ctx context.Context, req *connect.Request[zerxv1.List{{.Pascal}}sRequest]) (*connect.Response[zerxv1.List{{.Pascal}}sResponse], error) {
	ps, offset := normalizePage(req.Msg.GetPage().GetPage(), req.Msg.GetPage().GetPageSize())
	if kw := req.Msg.GetKeyword(); kw != "" {
		q := gorm.G[{{.Pascal}}](s.db).Where("name LIKE ?", "%"+kw+"%")
		total, err := q.Count(ctx, "id")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		rows, err := q.Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		return connect.NewResponse(&zerxv1.List{{.Pascal}}sResponse{Items: protoList(rows), Total: total}), nil
	}
	total, err := gorm.G[{{.Pascal}}](s.db).Count(ctx, "id")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	rows, err := gorm.G[{{.Pascal}}](s.db).Order("id ASC").Limit(ps).Offset(offset).Find(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&zerxv1.List{{.Pascal}}sResponse{Items: protoList(rows), Total: total}), nil
}

func (s *Service) Get{{.Pascal}}(ctx context.Context, req *connect.Request[zerxv1.Get{{.Pascal}}Request]) (*connect.Response[zerxv1.{{.Pascal}}], error) {
	row, err := gorm.G[{{.Pascal}}](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(toProto(row)), nil
}

func (s *Service) Create{{.Pascal}}(ctx context.Context, req *connect.Request[zerxv1.Create{{.Pascal}}Request]) (*connect.Response[zerxv1.{{.Pascal}}], error) {
	row := {{.Pascal}}{Name: req.Msg.GetName(){{range .Fields}}, {{.GoName}}: req.Msg.Get{{.GoName}}(){{end}}}
	if err := gorm.G[{{.Pascal}}](s.db).Create(ctx, &row); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": row.ID, "name": row.Name}}))
	return connect.NewResponse(toProto(row)), nil
}

func (s *Service) Update{{.Pascal}}(ctx context.Context, req *connect.Request[zerxv1.Update{{.Pascal}}Request]) (*connect.Response[zerxv1.{{.Pascal}}], error) {
	if err := s.db.WithContext(ctx).Model(&{{.Pascal}}{}).Where("id = ?", req.Msg.GetId()).Updates(map[string]any{
		"name": req.Msg.GetName(),
{{- range .Fields}}
		"{{.JSONName}}": req.Msg.Get{{.GoName}}(),
{{- end}}
	}).Error; err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	row, err := gorm.G[{{.Pascal}}](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"after": map[string]any{"id": row.ID, "name": row.Name}}))
	return connect.NewResponse(toProto(row)), nil
}

func (s *Service) Delete{{.Pascal}}(ctx context.Context, req *connect.Request[zerxv1.Delete{{.Pascal}}Request]) (*connect.Response[zerxv1.Delete{{.Pascal}}Response], error) {
	old, err := gorm.G[{{.Pascal}}](s.db).Where("id = ?", req.Msg.GetId()).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := gorm.G[{{.Pascal}}](s.db).Where("id = ?", req.Msg.GetId()).Delete(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	audit.Record(ctx, auditJSON(map[string]any{"before": map[string]any{"id": old.ID, "name": old.Name}}))
	return connect.NewResponse(&zerxv1.Delete{{.Pascal}}Response{}), nil
}

func protoList(rows []{{.Pascal}}) []*zerxv1.{{.Pascal}} {
	out := make([]*zerxv1.{{.Pascal}}, 0, len(rows))
	for i := range rows {
		out = append(out, toProto(rows[i]))
	}
	return out
}

func toProto(r {{.Pascal}}) *zerxv1.{{.Pascal}} {
	return &zerxv1.{{.Pascal}}{
		Id:        r.ID,
		Name:      r.Name,
{{- range .Fields}}
		{{.GoName}}: r.{{.GoName}},
{{- end}}
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

func normalizePage(page, pageSize int32) (ps, offset int) {
	p := int(page)
	if p < 1 {
		p = 1
	}
	ps = int(pageSize)
	if ps < 1 || ps > 200 {
		ps = 20
	}
	return ps, (p - 1) * ps
}

func auditJSON(v map[string]any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
`

const teardownTmpl = `-- Teardown for the "{{.Name}}" plugin (manual; v1 does no auto-teardown).
-- Run AFTER removing the plugin (proto + impl + all.go lines) and rebuilding.
-- Covers every residue surface so a clean removal (and a later same-name
-- reinstall) is safe. Underscores in the name prefix are escaped (ESCAPE '!')
-- so "{{.Name}}" cannot match a prefix-overlapping plugin (e.g. shop vs shopping).
--
-- 1. Owned data table(s) (add any extra plg_{{.Name}}_* tables you declared).
DROP TABLE IF EXISTS plg_{{.Name}}_{{.Name}}s;

-- 2. Menus + grants. Matches the group-heading row (exactly "plg_{{.Name}}")
--    AND its sub-pages ("plg_{{.Name}}_*"), plus their button/role grants.
DELETE FROM role_buttons WHERE button_id IN (SELECT id FROM menu_buttons WHERE menu_id IN (SELECT id FROM menus WHERE name = 'plg_{{.Name}}' OR name LIKE 'plg!_{{.Name}}!_%' ESCAPE '!'));
DELETE FROM menu_buttons WHERE menu_id IN (SELECT id FROM menus WHERE name = 'plg_{{.Name}}' OR name LIKE 'plg!_{{.Name}}!_%' ESCAPE '!');
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE name = 'plg_{{.Name}}' OR name LIKE 'plg!_{{.Name}}!_%' ESCAPE '!');
DELETE FROM menus WHERE name = 'plg_{{.Name}}' OR name LIKE 'plg!_{{.Name}}!_%' ESCAPE '!';

-- 3. API catalog + Casbin policies for the plugin's services (all are named
--    with the plugin's PascalCase prefix, so one prefix covers every service).
DELETE FROM apis WHERE procedure LIKE '/zerx.v1.{{.Pascal}}%';
DELETE FROM casbin_rule WHERE v1 LIKE '/zerx.v1.{{.Pascal}}%';

-- 4. Scheduled jobs + their run history for the plugin's handlers (key prefix
--    "{{.Name}}_"). Delete executions first so the subquery still resolves ids.
DELETE FROM job_executions WHERE job_id IN (SELECT id FROM scheduled_jobs WHERE handler LIKE '{{.Name}}!_%' ESCAPE '!');
DELETE FROM scheduled_jobs WHERE handler LIKE '{{.Name}}!_%' ESCAPE '!';

-- 5. gormigrate ledger rows (else a same-name reinstall SKIPS schema creation
--    and leaves a missing table).
DELETE FROM migrations WHERE id LIKE 'plg!_{{.Name}}!_%' ESCAPE '!';

-- 6. Runtime enable/disable state (else a reinstall could boot disabled).
DELETE FROM plugin_states WHERE name = '{{.Name}}';
`

// i18nTmpl is the plugin's self-contained translations, collected at build time
// by web/src/lib/i18n.tsx and merged under the plg.{{.Name}} namespace. ZIP
// install unpacks this file with the rest of web/, so a packaged plugin's menu
// titles and page strings resolve with no edit to i18n.tsx. Keep en/zh keys
// identical and add a key here for every t("plg.{{.Name}}.<key>") you use.
const i18nTmpl = `export default {
  en: {
    title: "{{.Pascal}}",
  },
  zh: {
    title: "{{.Pascal}}",
  },
};
`

const webTmpl = `import { useQuery } from "@connectrpc/connect-query";
import { useState } from "react";

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
import { list{{.Pascal}}s } from "@/gen/zerx/v1/{{.Name}}-{{.Pascal}}Service_connectquery";
import { useI18n } from "@/lib/i18n";

// {{.Pascal}} is the {{.Name}} plugin page, loaded dynamically by /p/$name.
export default function {{.Pascal}}() {
  const { t } = useI18n();
  const [keyword, setKeyword] = useState("");
  const { data, isPending } = useQuery(list{{.Pascal}}s, {
    page: { page: 1, pageSize: 50 },
    keyword,
  });
  const items = data?.items ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">{t("plg.{{.Name}}.title")}</h1>
      </div>
      <Input
        value={keyword}
        onChange={(e) => setKeyword(e.target.value)}
        placeholder={t("common.search")}
        className="max-w-xs"
      />
      <Card className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>ID</TableHead>
              <TableHead>{t("common.name")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isPending ? (
              <TableRow>
                <TableCell colSpan={2}>
                  <Skeleton className="h-8 w-full" />
                </TableCell>
              </TableRow>
            ) : (
              items.map((it) => (
                <TableRow key={String(it.id)}>
                  <TableCell>{String(it.id)}</TableCell>
                  <TableCell>{it.name}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
}
`
