package dbcore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/internal/config"
	logger "github.com/komari-monitor/komari/utils/log"
	"gorm.io/gorm"
)

const (
	// MySQLBackupFileName is the snapshot filename embedded in backup zip when DB type is mysql.
	MySQLBackupFileName = "komari-mysql-backup.json"

	mysqlBackupFormatVersion = 1
)

type mysqlBackupTableDescriptor struct {
	Name       string
	NewSlice   func() any
	ForeignKey bool
}

type mysqlBackupTableDump struct {
	Name string          `json:"name"`
	Rows json.RawMessage `json:"rows"`
}

var mysqlBackupTables = []mysqlBackupTableDescriptor{
	{Name: "configs", NewSlice: func() any { return &[]config.ConfigItem{} }},
	{Name: "users", NewSlice: func() any { return &[]models.User{} }},
	{Name: "clients", NewSlice: func() any { return &[]models.Client{} }},
	{Name: "logs", NewSlice: func() any { return &[]models.Log{} }},
	{Name: "clipboards", NewSlice: func() any { return &[]models.Clipboard{} }},
	{Name: "load_notifications", NewSlice: func() any { return &[]models.LoadNotification{} }},
	{Name: "offline_notifications", NewSlice: func() any { return &[]models.OfflineNotification{} }, ForeignKey: true},
	{Name: "traffic_report_notifications", NewSlice: func() any { return &[]models.TrafficReportNotification{} }, ForeignKey: true},
	{Name: "ping_tasks", NewSlice: func() any { return &[]models.PingTask{} }},
	{Name: "oidc_providers", NewSlice: func() any { return &[]models.OidcProvider{} }},
	{Name: "message_sender_providers", NewSlice: func() any { return &[]models.MessageSenderProvider{} }},
	{Name: "theme_configurations", NewSlice: func() any { return &[]models.ThemeConfiguration{} }},
	{Name: "plugin_configurations", NewSlice: func() any { return &[]models.PluginConfiguration{} }},
	{Name: "sessions", NewSlice: func() any { return &[]models.Session{} }, ForeignKey: true},
	{Name: "tasks", NewSlice: func() any { return &[]models.Task{} }},
	{Name: "task_results", NewSlice: func() any { return &[]models.TaskResult{} }, ForeignKey: true},
	{Name: "persistent_files", NewSlice: func() any { return &[]models.PersistentFile{} }},
	{Name: "persistent_file_chunks", NewSlice: func() any { return &[]models.PersistentFileChunk{} }, ForeignKey: true},
}

var mysqlBackupRestoreOrder = []string{
	"configs",
	"users",
	"clients",
	"logs",
	"clipboards",
	"load_notifications",
	"offline_notifications",
	"traffic_report_notifications",
	"ping_tasks",
	"oidc_providers",
	"message_sender_providers",
	"theme_configurations",
	"plugin_configurations",
	"sessions",
	"tasks",
	"task_results",
	"persistent_files",
	"persistent_file_chunks",
}

func ExportMySQLBackupToFile(path string) error {
	return exportMySQLBackupToFile(GetDBInstance(), path)
}

func ImportMySQLBackupFromFile(path string) error {
	return importMySQLBackupFromFile(GetDBInstance(), path)
}

func exportMySQLBackupToFile(db *gorm.DB, path string) error {
	if db.Dialector.Name() != "mysql" {
		return fmt.Errorf("mysql backup is only supported for mysql, got %s", db.Dialector.Name())
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create backup directory failed: %w", err)
	}

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp mysql backup file failed: %w", err)
	}

	writeErr := writeMySQLBackup(db, f)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp mysql backup file failed: %w", closeErr)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename mysql backup file failed: %w", err)
	}
	return nil
}

func writeMySQLBackup(db *gorm.DB, w io.Writer) error {
	if _, err := io.WriteString(w, `{"version":`+strconv.Itoa(mysqlBackupFormatVersion)+`,"created_at":`); err != nil {
		return fmt.Errorf("write backup header failed: %w", err)
	}
	createdAt, err := json.Marshal(time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("marshal backup timestamp failed: %w", err)
	}
	if _, err := w.Write(createdAt); err != nil {
		return fmt.Errorf("write backup timestamp failed: %w", err)
	}
	if _, err := io.WriteString(w, `,"tables":[`); err != nil {
		return fmt.Errorf("write tables header failed: %w", err)
	}

	emitted := false
	for _, desc := range mysqlBackupTables {
		if !db.Migrator().HasTable(desc.Name) {
			continue
		}
		if emitted {
			if _, err := io.WriteString(w, ","); err != nil {
				return fmt.Errorf("write table delimiter failed: %w", err)
			}
		}
		emitted = true

		rows := desc.NewSlice()
		if err := db.Table(desc.Name).Find(rows).Error; err != nil {
			return fmt.Errorf("query table %s failed: %w", desc.Name, err)
		}
		rowsJSON, err := json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("marshal table %s failed: %w", desc.Name, err)
		}

		tableDump := mysqlBackupTableDump{
			Name: desc.Name,
			Rows: json.RawMessage(rowsJSON),
		}
		tableJSON, err := json.Marshal(tableDump)
		if err != nil {
			return fmt.Errorf("marshal table dump %s failed: %w", desc.Name, err)
		}
		if _, err := w.Write(tableJSON); err != nil {
			return fmt.Errorf("write table dump %s failed: %w", desc.Name, err)
		}
	}

	if _, err := io.WriteString(w, "]}"); err != nil {
		return fmt.Errorf("write backup footer failed: %w", err)
	}
	return nil
}

func importMySQLBackupFromFile(db *gorm.DB, path string) error {
	if db.Dialector.Name() != "mysql" {
		return fmt.Errorf("mysql restore is only supported for mysql, got %s", db.Dialector.Name())
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open mysql backup file failed: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)

	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read mysql backup root token failed: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return errors.New("invalid mysql backup format: root must be object")
	}

	var foundTables bool
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return fmt.Errorf("read mysql backup object key failed: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("invalid mysql backup format: object key must be string")
		}

		switch key {
		case "version":
			var version int
			if err := dec.Decode(&version); err != nil {
				return fmt.Errorf("decode backup version failed: %w", err)
			}
			if version != mysqlBackupFormatVersion {
				logger.Warn("dbcore", "MySQL backup version differs", "file", version, "expected", mysqlBackupFormatVersion)
			}
		case "created_at":
			var createdAt string
			if err := dec.Decode(&createdAt); err != nil {
				return fmt.Errorf("decode backup timestamp failed: %w", err)
			}
		case "tables":
			if err := importMySQLBackupTables(db, dec); err != nil {
				return err
			}
			foundTables = true
		default:
			var discard any
			if err := dec.Decode(&discard); err != nil {
				return fmt.Errorf("discard unknown backup field %s failed: %w", key, err)
			}
		}
	}

	endToken, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read mysql backup object end failed: %w", err)
	}
	if delim, ok := endToken.(json.Delim); !ok || delim != '}' {
		return errors.New("invalid mysql backup format: malformed object ending")
	}

	if !foundTables {
		return errors.New("invalid mysql backup format: missing tables field")
	}
	return nil
}

// ExportMySQLBackup writes the portable primary-database snapshot directly to
// w. It is used by both downloadable backups and upgrade snapshots.
func ExportMySQLBackup(w io.Writer) error {
	db := GetDBInstance()
	if db.Dialector.Name() != "mysql" {
		return fmt.Errorf("mysql backup is only supported for mysql, got %s", db.Dialector.Name())
	}
	return writeMySQLBackup(db, w)
}

func importMySQLBackupTables(db *gorm.DB, dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read tables array start failed: %w", err)
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return errors.New("invalid mysql backup format: tables must be array")
	}

	descriptors := make(map[string]mysqlBackupTableDescriptor, len(mysqlBackupTables))
	for _, desc := range mysqlBackupTables {
		descriptors[desc.Name] = desc
	}

	dumps := make(map[string]json.RawMessage, len(mysqlBackupTables))
	for dec.More() {
		var tableDump mysqlBackupTableDump
		if err := dec.Decode(&tableDump); err != nil {
			return fmt.Errorf("decode table dump failed: %w", err)
		}
		if _, ok := descriptors[tableDump.Name]; !ok {
			logger.Warn("dbcore", "Skipping unknown MySQL backup table", "table", tableDump.Name)
			continue
		}
		dumps[tableDump.Name] = tableDump.Rows
	}

	endToken, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read tables array end failed: %w", err)
	}
	if delim, ok := endToken.(json.Delim); !ok || delim != ']' {
		return errors.New("invalid mysql backup format: malformed tables array ending")
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		for i := len(mysqlBackupRestoreOrder) - 1; i >= 0; i-- {
			desc, ok := descriptors[mysqlBackupRestoreOrder[i]]
			if !ok || !tx.Migrator().HasTable(desc.Name) {
				continue
			}
			if err := tx.Exec("DELETE FROM `" + desc.Name + "`").Error; err != nil {
				return fmt.Errorf("clear table %s failed: %w", desc.Name, err)
			}
		}

		for _, name := range mysqlBackupRestoreOrder {
			desc, ok := descriptors[name]
			if !ok || desc.ForeignKey {
				continue
			}
			if !tx.Migrator().HasTable(desc.Name) {
				continue
			}
			raw := bytes.TrimSpace(dumps[name])
			if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
				continue
			}

			rows := desc.NewSlice()
			if err := json.Unmarshal(raw, rows); err != nil {
				return fmt.Errorf("decode rows for table %s failed: %w", desc.Name, err)
			}
			if sliceLength(rows) == 0 {
				continue
			}
			if err := tx.Table(desc.Name).CreateInBatches(rows, 500).Error; err != nil {
				return fmt.Errorf("insert rows into %s failed: %w", desc.Name, err)
			}
		}
		for _, name := range mysqlBackupRestoreOrder {
			desc, ok := descriptors[name]
			if !ok || !desc.ForeignKey || !tx.Migrator().HasTable(desc.Name) {
				continue
			}
			raw := bytes.TrimSpace(dumps[name])
			if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
				continue
			}
			rows := desc.NewSlice()
			if err := json.Unmarshal(raw, rows); err != nil {
				return fmt.Errorf("decode rows for table %s: %w", desc.Name, err)
			}
			if sliceLength(rows) > 0 {
				if err := tx.Table(desc.Name).CreateInBatches(rows, 500).Error; err != nil {
					return fmt.Errorf("insert rows into %s: %w", desc.Name, err)
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func sliceLength(v any) int {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return 0
	}
	return rv.Len()
}

func restoreMySQLBackupIfPresent(db *gorm.DB, backupPath string) error {
	if db.Dialector.Name() != "mysql" {
		return nil
	}

	if _, err := os.Stat(backupPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat mysql backup snapshot failed: %w", err)
	}

	logger.Infof("dbcore", "[restore] MySQL backup snapshot detected: %s", backupPath)
	if err := os.MkdirAll("./data/backup", 0o755); err != nil {
		logger.Errorf("dbcore", "[restore] failed to create backup directory for MySQL pre-restore snapshot: %v", err)
	} else {
		preRestore := filepath.Join(
			"./data/backup",
			fmt.Sprintf("%s-mysql-pre-restore.json", time.Now().Format("20060102-150405")),
		)
		if err := exportMySQLBackupToFile(db, preRestore); err != nil {
			logger.Errorf("dbcore", "[restore] failed to export MySQL pre-restore snapshot: %v", err)
		} else {
			logger.Infof("dbcore", "[restore] MySQL pre-restore snapshot saved to %s", preRestore)
		}
	}

	if err := importMySQLBackupFromFile(db, backupPath); err != nil {
		return fmt.Errorf("import mysql backup snapshot failed: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		logger.Errorf("dbcore", "[restore] MySQL backup imported but failed to remove snapshot file: %v", err)
	} else {
		logger.Infof("dbcore", "[restore] MySQL backup snapshot imported and removed")
	}
	return nil
}
