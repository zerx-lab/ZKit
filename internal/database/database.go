// Package database opens the configured GORM data source and provides
// migration and seeding helpers. All supported drivers are pure-Go (CGO-free).
package database

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/zerx-lab/zkit/internal/config"
)

// Open connects to the configured data source. The driver is selected by
// cfg.Driver; only GORM's bundled dialectors are used. The env controls the
// GORM logger verbosity.
func Open(cfg config.DBConfig, env string) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported db driver %q (want sqlite|postgres|mysql)", cfg.Driver)
	}

	level := logger.Error
	if env == "dev" {
		level = logger.Warn
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(level),
	})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.Driver, err)
	}

	return db, nil
}
