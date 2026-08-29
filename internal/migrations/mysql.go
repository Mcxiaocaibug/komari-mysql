package migrations

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// prepareMySQLPrimaryKeys upgrades legacy fork tables before GORM's normal
// AutoMigrate pass. MySQL can otherwise attempt to recreate an existing
// unique index while adding a surrogate key and fail on every restart.
func prepareMySQLPrimaryKeys(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "mysql" {
		return nil
	}
	for _, table := range []string{"offline_notifications", "traffic_report_notifications"} {
		if !db.Migrator().HasTable(table) || !hasTableColumn(db, table, "client") {
			continue
		}
		primary, err := mysqlPrimaryKeyColumns(db, table)
		if err != nil {
			return err
		}
		if len(primary) == 1 && strings.EqualFold(primary[0], "client") {
			continue
		}
		alter := make([]string, 0, 4)
		if len(primary) > 0 {
			alter = append(alter, "DROP PRIMARY KEY")
		}
		if hasTableColumn(db, table, "id") {
			alter = append(alter, "DROP COLUMN `id`")
		}
		alter = append(alter, "MODIFY `client` varchar(36) NOT NULL", "ADD PRIMARY KEY (`client`)")
		// Keep the whole shape change in one ALTER. MySQL rejects an intermediate
		// state where an AUTO_INCREMENT id has lost its key but has not yet been
		// dropped.
		if err := db.Exec("ALTER TABLE `" + table + "` " + strings.Join(alter, ", ")).Error; err != nil {
			return fmt.Errorf("prepare MySQL primary key for %s: %w", table, err)
		}
	}
	return nil
}

func mysqlPrimaryKeyColumns(db *gorm.DB, table string) ([]string, error) {
	var rows []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
	}
	err := db.Raw(`SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
		ORDER BY ORDINAL_POSITION`, table).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	columns := make([]string, len(rows))
	for index, row := range rows {
		columns[index] = row.ColumnName
	}
	return columns, nil
}
