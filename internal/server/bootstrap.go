package server

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/cmd/flags"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/metricstore"
	"github.com/komari-monitor/komari/pkg/metric"
	"github.com/komari-monitor/komari/utils"
)

// Bootstrap initializes the data directory, primary database, and settings.
func (a *App) Bootstrap() error {
	if err := os.MkdirAll("./data/theme", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create theme directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}
	if err := os.MkdirAll("./data/plugin-data", os.ModePerm); err != nil {
		return fmt.Errorf("failed to create plugin storage directory: %w", err)
	}

	dbcore.SetVersionID(utils.CurrentVersion + "-" + utils.VersionHash)
	if err := dbcore.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	a.dbReady = true
	a.addCleanup("database", func(context.Context) error { return dbcore.Close() })
	if err := defaultMetricStoreToPrimaryMySQL(); err != nil {
		return fmt.Errorf("failed to configure MySQL metric storage: %w", err)
	}
	if err := dbcore.RestorePersistentFiles(context.Background(), "./data"); err != nil {
		return fmt.Errorf("failed to restore MySQL-backed files: %w", err)
	}
	if flags.IsMySQL() {
		if err := dbcore.SyncPersistentFiles(context.Background(), "./data"); err != nil {
			return fmt.Errorf("failed to initialize MySQL file mirror: %w", err)
		}
		a.addCleanup("mysql-file-mirror", func(ctx context.Context) error {
			return dbcore.SyncPersistentFilesWithTimeout(ctx, "./data")
		})
	}

	gin.SetMode(gin.ReleaseMode)
	settings, err := config.GetManyAs[config.Settings]()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}
	a.settings = settings
	return nil
}

// defaultMetricStoreToPrimaryMySQL makes a new MySQL deployment fully MySQL
// backed. Existing installations keep their explicitly configured independent
// Metric Store target and can still migrate it through the upstream UI.
func defaultMetricStoreToPrimaryMySQL() error {
	if !flags.IsMySQL() {
		return nil
	}
	persisted, err := metricstore.PersistedConfig()
	if err != nil {
		return err
	}
	if persisted {
		return nil
	}
	dsn, err := dbcore.MySQLDSN()
	if err != nil {
		return err
	}
	return config.SetMany(map[string]any{
		metricstore.MetricDBDriverKey: string(metric.DriverMySQL),
		metricstore.MetricDBDSNKey:    dsn,
	})
}
