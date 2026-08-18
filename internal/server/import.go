package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	casbin "github.com/casbin/casbin/v3"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/gen/go/zerx/v1/zerxv1connect"
	"github.com/zerx-lab/zkit/internal/auth"
	"github.com/zerx-lab/zkit/internal/model"
)

type importRowError struct {
	Row    int    `json:"row"`
	Reason string `json:"reason"`
}

type importSummary struct {
	Created int              `json:"created"`
	Failed  []importRowError `json:"failed"`
}

// importUsersTemplateHandler returns an .xlsx template for user import.
func importUsersTemplateHandler(issuer *auth.Issuer, enforcer *casbin.SyncedCachedEnforcer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorizeHTTP(w, r, issuer, enforcer, zerxv1connect.UserServiceCreateUserProcedure); !ok {
			return
		}
		f := excelize.NewFile()
		defer func() { _ = f.Close() }()
		const sheet = "Sheet1"
		setRow(f, sheet, 1, "email", "name", "password", "roles", "nickname", "phone")
		setRow(f, sheet, 2, "user@example.com", "示例用户", "Passw0rd!", "user;admin", "昵称", "13800000000")
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", "attachment; filename=users_template.xlsx")
		_ = f.Write(w)
	}
}

// importUsersHandler bulk-creates users from an uploaded .xlsx. Each row is
// validated independently; failures are reported per-row without aborting.
func importUsersHandler(issuer *auth.Issuer, enforcer *casbin.SyncedCachedEnforcer, db *gorm.DB, policy *auth.Policy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := authorizeHTTP(w, r, issuer, enforcer, zerxv1connect.UserServiceCreateUserProcedure); !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()

		xf, err := excelize.OpenReader(file)
		if err != nil {
			http.Error(w, "invalid xlsx", http.StatusBadRequest)
			return
		}
		defer func() { _ = xf.Close() }()

		sheet := xf.GetSheetName(0)
		rows, err := xf.GetRows(sheet)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		validRoles, err := allRoleCodes(ctx, db)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		summary := importSummary{Failed: []importRowError{}}
		for i, row := range rows {
			if i == 0 {
				continue // header
			}
			if len(row) == 0 || strings.TrimSpace(at(row, 0)) == "" {
				continue
			}
			email := strings.TrimSpace(at(row, 0))
			name := strings.TrimSpace(at(row, 1))
			password := at(row, 2)
			rolesRaw := at(row, 3)
			nickname := at(row, 4)
			phone := at(row, 5)

			if reason := importOne(ctx, db, policy, validRoles, email, name, password, rolesRaw, nickname, phone); reason != "" {
				summary.Failed = append(summary.Failed, importRowError{Row: i + 1, Reason: reason})
				continue
			}
			summary.Created++
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summary)
	}
}

func importOne(ctx context.Context, db *gorm.DB, policy *auth.Policy, validRoles map[string]bool, email, name, password, rolesRaw, nickname, phone string) string {
	if !strings.Contains(email, "@") {
		return "邮箱格式无效"
	}
	if name == "" {
		return "姓名不能为空"
	}
	if err := policy.Validate(password); err != nil {
		return err.Error()
	}
	roles := splitRoles(rolesRaw)
	if len(roles) == 0 {
		return "至少需要一个角色"
	}
	for _, rc := range roles {
		if !validRoles[rc] {
			return "角色不存在: " + rc
		}
	}
	if _, err := gorm.G[model.User](db).Where("email = ?", email).First(ctx); err == nil {
		return "邮箱已存在"
	}
	hash, err := auth.Hash(password)
	if err != nil {
		return "密码加密失败"
	}
	u := model.User{Email: email, Name: name, Nickname: nickname, Phone: phone, PasswordHash: hash, Status: true}
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		ur := make([]model.UserRole, 0, len(roles))
		for _, rc := range roles {
			ur = append(ur, model.UserRole{UserID: u.ID, RoleCode: rc})
		}
		return tx.CreateInBatches(&ur, 100).Error
	})
	if txErr != nil {
		return "创建失败"
	}
	return ""
}

func allRoleCodes(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	roles, err := gorm.G[model.Role](db).Find(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(roles))
	for i := range roles {
		out[roles[i].Code] = true
	}
	return out, nil
}

func splitRoles(s string) []string {
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func at(row []string, i int) string {
	if i < len(row) {
		return row[i]
	}
	return ""
}
