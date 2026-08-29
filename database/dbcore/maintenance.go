package dbcore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/komari-monitor/komari/cmd/flags"
)

// DriverName returns the active primary database driver.
func DriverName() string {
	return flags.NormalizeDatabaseType(flags.DatabaseType)
}

var maintenanceMu sync.Mutex

// StorageSize returns the bytes occupied by the main SQLite database and its
// WAL/SHM sidecar files.
func StorageSize() (int64, error) {
	if !flags.IsSQLite() {
		return 0, errors.New("main database size is only available for SQLite")
	}
	return sqliteFileSetSize(resolveDatabaseFile())
}

// ReclaimSpace checkpoints the main database WAL and rewrites the SQLite file.
func ReclaimSpace(ctx context.Context) error {
	maintenanceMu.Lock()
	defer maintenanceMu.Unlock()

	db, err := GetDBInstance().DB()
	if err != nil {
		return fmt.Errorf("get main database connection: %w", err)
	}
	if flags.IsSQLite() {
		if err := checkpointSQLiteWAL(ctx, db); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			return fmt.Errorf("vacuum main database: %w", err)
		}
		return checkpointSQLiteWAL(ctx, db)
	}
	if flags.IsMySQL() {
		rows, err := db.QueryContext(ctx, "SHOW TABLES")
		if err != nil {
			return fmt.Errorf("list MySQL main database tables: %w", err)
		}
		var tables []string
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan MySQL main database table: %w", err)
			}
			tables = append(tables, "`"+strings.ReplaceAll(table, "`", "``")+"`")
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate MySQL main database tables: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close MySQL main database table list: %w", err)
		}
		if len(tables) == 0 {
			return nil
		}
		if _, err := db.ExecContext(ctx, "OPTIMIZE TABLE "+strings.Join(tables, ", ")); err != nil {
			return fmt.Errorf("optimize MySQL main database: %w", err)
		}
		return nil
	}
	return fmt.Errorf("main database maintenance is unsupported for %s", DriverName())
}

func checkpointSQLiteWAL(ctx context.Context, db *sql.DB) error {
	var busy, logFrames, checkpointedFrames int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return fmt.Errorf("checkpoint main database WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf(
			"checkpoint main database WAL: database is busy (%d log frames, %d checkpointed)",
			logFrames,
			checkpointedFrames,
		)
	}
	return nil
}

func sqliteFileSetSize(dsn string) (int64, error) {
	path := strings.TrimPrefix(strings.TrimSpace(dsn), "file:")
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	if path == "" || path == ":memory:" || strings.Contains(strings.ToLower(dsn), "mode=memory") {
		return 0, errors.New("database is not backed by a local file")
	}

	var total int64
	foundDatabase := false
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		switch {
		case err == nil:
			total += info.Size()
			if suffix == "" {
				foundDatabase = true
			}
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return 0, fmt.Errorf("stat database file %q: %w", path+suffix, err)
		}
	}
	if !foundDatabase {
		return 0, fmt.Errorf("database file %q does not exist", path)
	}
	return total, nil
}
