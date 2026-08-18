package database

import (
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/zerx-lab/zkit/internal/model"
)

// Migrate runs versioned schema migrations via gormigrate. The migration record
// table is "migrations". 0001_baseline AutoMigrates the full model snapshot of
// this release (incremental: builds a fresh DB and backfills missing tables /
// columns on an existing one). 0002/0003 migrate the legacy single-role column
// into the user_roles table, guarded by HasColumn so a fresh DB skips them.
//
// casbin_rule is never listed here: the gorm-adapter inside casbin.New
// auto-migrates it (later, in server.New), independent of the migrations table.
// extra holds plugin-supplied migrations appended after the core 0001-0007 set;
// cmd/server/main.go passes plugin.CollectMigrations() (database does not import
// the plugin package, avoiding the database->plugin->jobs import cycle).
func Migrate(db *gorm.DB, extra []*gormigrate.Migration) error {
	migrations := []*gormigrate.Migration{
		{
			ID: "0001_baseline",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(
					&model.User{},
					&model.UserRole{},
					&model.Role{},
					&model.Menu{},
					&model.MenuButton{},
					&model.RoleMenu{},
					&model.RoleButton{},
					&model.API{},
					&model.Dictionary{},
					&model.DictionaryItem{},
					&model.OperationLog{},
					&model.LoginLog{},
					&model.UserSession{},
					&model.SysParam{},
					&model.File{},
					&model.PasswordResetToken{},
					&model.PasswordHistory{},
					&model.UserTOTP{},
					&model.TOTPRecoveryCode{},
					&model.ScheduledJob{},
					&model.JobExecution{},
					&model.JobLock{},
					&model.CaptchaCode{},
					&model.LoginAttempt{},
					&model.PluginState{},
				)
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0002_backfill_user_roles",
			Migrate: func(tx *gorm.DB) error {
				if !tx.Migrator().HasColumn("users", "role") {
					return nil // fresh DB: no legacy column to backfill
				}
				type legacyUser struct {
					ID   uint64
					Role string
				}
				var rows []legacyUser
				if err := tx.Table("users").Select("id", "role").Find(&rows).Error; err != nil {
					return err
				}
				batch := make([]model.UserRole, 0, len(rows))
				for _, r := range rows {
					if r.Role != "" {
						batch = append(batch, model.UserRole{UserID: r.ID, RoleCode: r.Role})
					}
				}
				if len(batch) == 0 {
					return nil
				}
				return tx.CreateInBatches(&batch, 100).Error
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0003_drop_users_role",
			Migrate: func(tx *gorm.DB) error {
				if tx.Migrator().HasColumn("users", "role") {
					return tx.Migrator().DropColumn("users", "role")
				}
				return nil
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0004_file_visibility",
			Migrate: func(tx *gorm.DB) error {
				// Existing DB: add the visibility column (NOT NULL DEFAULT 'private',
				// so pre-existing rows initially become 'private'). A fresh DB already
				// has the column from 0001's AutoMigrate(&model.File{}).
				if err := tx.AutoMigrate(&model.File{}); err != nil {
					return err
				}
				// Grandfather: this migration runs exactly once, so the unconditional
				// UPDATE only ever touches rows that existed at migration time, keeping
				// their current "anonymously reachable" semantics. A fresh DB hits 0 rows.
				return tx.Model(&model.File{}).Where("1 = 1").Update("visibility", model.VisibilityPublic).Error
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0005_shared_state",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&model.JobLock{}, &model.CaptchaCode{}, &model.LoginAttempt{})
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0006_plugin_states",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&model.PluginState{})
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
		{
			ID: "0007_plugin_menu_after_dashboard",
			Migrate: func(tx *gorm.DB) error {
				// Open a gap after dashboard so plugin top-level menus (old seed
				// default 50) sit between dashboard and system/audit. Only rewrite
				// rows still at original seed values so admin custom sorts survive.
				if err := tx.Model(&model.Menu{}).
					Where("name = ? AND parent_id = ? AND sort = ?", "system", 0, 2).
					Update("sort", 10).Error; err != nil {
					return err
				}
				if err := tx.Model(&model.Menu{}).
					Where("name = ? AND parent_id = ? AND sort = ?", "audit", 0, 3).
					Update("sort", 20).Error; err != nil {
					return err
				}
				var tops []model.Menu
				if err := tx.Where("parent_id = ?", 0).Find(&tops).Error; err != nil {
					return err
				}
				for i := range tops {
					if strings.HasPrefix(tops[i].Name, "plg_") && tops[i].Sort == 50 {
						if err := tx.Model(&model.Menu{}).Where("id = ?", tops[i].ID).Update("sort", 2).Error; err != nil {
							return err
						}
					}
				}
				return nil
			},
			Rollback: func(*gorm.DB) error { return nil },
		},
	}

	migrations = append(migrations, extra...)

	return gormigrate.New(db, gormigrate.DefaultOptions, migrations).Migrate()
}
