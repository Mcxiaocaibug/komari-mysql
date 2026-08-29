package dbcore

import (
	"strings"
	"testing"

	"github.com/komari-monitor/komari/cmd/flags"
)

func TestMySQLDSNPrefersExplicitValue(t *testing.T) {
	original := snapshotDatabaseFlags()
	defer original.restore()

	flags.DatabaseDSN = "USER:PASSWORD@tcp(HOST:3306)/komari?parseTime=true"
	got, err := MySQLDSN()
	if err != nil {
		t.Fatalf("MySQLDSN: %v", err)
	}
	if got != flags.DatabaseDSN {
		t.Fatalf("MySQLDSN() = %q, want explicit DSN", got)
	}
}

func TestMySQLDSNBuildsUTCParseTimeConfiguration(t *testing.T) {
	original := snapshotDatabaseFlags()
	defer original.restore()

	flags.DatabaseDSN = ""
	flags.DatabaseUser = "komari"
	flags.DatabasePass = "p@ss:word"
	flags.DatabaseHost = "2001:db8::1"
	flags.DatabasePort = "3306"
	flags.DatabaseName = "komari"

	got, err := MySQLDSN()
	if err != nil {
		t.Fatalf("MySQLDSN: %v", err)
	}
	for _, part := range []string{
		"komari:p@ss:word@tcp([2001:db8::1]:3306)/komari?",
		"parseTime=true",
		"charset=utf8mb4",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("MySQLDSN() = %q, missing %q", got, part)
		}
	}
}

type databaseFlagSnapshot struct {
	dsn, host, port, user, pass, name string
}

func snapshotDatabaseFlags() databaseFlagSnapshot {
	return databaseFlagSnapshot{
		dsn: flags.DatabaseDSN, host: flags.DatabaseHost, port: flags.DatabasePort,
		user: flags.DatabaseUser, pass: flags.DatabasePass, name: flags.DatabaseName,
	}
}

func (s databaseFlagSnapshot) restore() {
	flags.DatabaseDSN, flags.DatabaseHost, flags.DatabasePort = s.dsn, s.host, s.port
	flags.DatabaseUser, flags.DatabasePass, flags.DatabaseName = s.user, s.pass, s.name
}
