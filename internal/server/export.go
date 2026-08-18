package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	casbin "github.com/casbin/casbin/v3"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
)

// exportHandler streams an .xlsx export for users / operation-logs / login-logs
// / error-logs. It performs manual JWT auth and Casbin authorization against the
// corresponding List procedure (mirroring the interceptor rules).
func exportHandler(issuer *auth.Issuer, enforcer *casbin.SyncedCachedEnforcer, db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resource := strings.TrimPrefix(r.URL.Path, "/api/export/")

		var proc string
		switch resource {
		case "users":
			proc = zerxv1connect.UserServiceListUsersProcedure
		case "operation-logs", "error-logs":
			proc = zerxv1connect.LogServiceListOperationLogsProcedure
		case "login-logs":
			proc = zerxv1connect.LogServiceListLoginLogsProcedure
		default:
			http.Error(w, "unknown export", http.StatusNotFound)
			return
		}
		if _, ok := authorizeHTTP(w, r, issuer, enforcer, proc); !ok {
			return
		}

		f := excelize.NewFile()
		defer func() { _ = f.Close() }()
		const sheet = "Sheet1"

		var err error
		switch resource {
		case "users":
			err = writeUsersSheet(r.Context(), db, f, sheet)
		case "login-logs":
			err = writeLoginLogsSheet(r.Context(), db, f, sheet, r)
		default: // operation-logs, error-logs
			err = writeOperationLogsSheet(r.Context(), db, f, sheet, r, resource == "error-logs")
		}
		if err != nil {
			http.Error(w, "export failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", "attachment; filename="+resource+".xlsx")
		_ = f.Write(w)
	}
}

func setRow(f *excelize.File, sheet string, row int, vals ...any) {
	cell, _ := excelize.CoordinatesToCellName(1, row)
	_ = f.SetSheetRow(sheet, cell, &vals)
}

func writeUsersSheet(ctx context.Context, db *gorm.DB, f *excelize.File, sheet string) error {
	setRow(f, sheet, 1, "ID", "Email", "Name", "Nickname", "Phone", "Roles", "Status", "CreatedAt")
	users, err := gorm.G[model.User](db).Order("id ASC").Find(ctx)
	if err != nil {
		return err
	}
	roleMap, err := allUserRoles(ctx, db)
	if err != nil {
		return err
	}
	for i := range users {
		u := users[i]
		setRow(f, sheet, i+2, u.ID, u.Email, u.Name, u.Nickname, u.Phone,
			strings.Join(roleMap[u.ID], ";"), u.Status, u.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func allUserRoles(ctx context.Context, db *gorm.DB) (map[uint64][]string, error) {
	rows, err := gorm.G[model.UserRole](db).Find(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[uint64][]string)
	for i := range rows {
		out[rows[i].UserID] = append(out[rows[i].UserID], rows[i].RoleCode)
	}
	return out, nil
}

func writeOperationLogsSheet(ctx context.Context, db *gorm.DB, f *excelize.File, sheet string, r *http.Request, errorsOnly bool) error {
	setRow(f, sheet, 1, "ID", "UserEmail", "Procedure", "Method", "IP", "Status", "Error", "LatencyMS", "CreatedAt")
	q := db.WithContext(ctx).Model(&model.OperationLog{})
	if errorsOnly {
		q = q.Where("status <> ?", "ok")
	}
	q = applyLogFilters(q, r)
	var rows []model.OperationLog
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		o := rows[i]
		setRow(f, sheet, i+2, o.ID, o.UserEmail, o.Procedure, o.Method, o.IP, o.Status, o.Error, o.LatencyMS, o.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func writeLoginLogsSheet(ctx context.Context, db *gorm.DB, f *excelize.File, sheet string, r *http.Request) error {
	setRow(f, sheet, 1, "ID", "Email", "IP", "Success", "Error", "CreatedAt")
	q := db.WithContext(ctx).Model(&model.LoginLog{})
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("start_at")); err == nil {
		q = q.Where("created_at >= ?", t)
	}
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("end_at")); err == nil {
		q = q.Where("created_at <= ?", t)
	}
	var rows []model.LoginLog
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		l := rows[i]
		setRow(f, sheet, i+2, l.ID, l.Email, l.IP, l.Success, l.Error, l.CreatedAt.Format(time.RFC3339))
	}
	return nil
}

func applyLogFilters(q *gorm.DB, r *http.Request) *gorm.DB {
	if v := r.URL.Query().Get("status"); v != "" {
		q = q.Where("status = ?", v)
	}
	if v := r.URL.Query().Get("method"); v != "" {
		q = q.Where("method = ?", v)
	}
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("start_at")); err == nil {
		q = q.Where("created_at >= ?", t)
	}
	if t, err := time.Parse(time.RFC3339, r.URL.Query().Get("end_at")); err == nil {
		q = q.Where("created_at <= ?", t)
	}
	return q
}
