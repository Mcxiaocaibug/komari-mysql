package flags

import "testing"

func TestNormalizeDatabaseType(t *testing.T) {
	tests := map[string]string{
		"":          DatabaseTypeSQLite,
		"sqlite":    DatabaseTypeSQLite,
		" SQLite ":  DatabaseTypeSQLite,
		"SQLITE":    DatabaseTypeSQLite,
		"mysql":     DatabaseTypeMySQL,
		" MySQL ":   DatabaseTypeMySQL,
		"postgres":  "postgres",
		" postgres": "postgres",
	}

	for input, want := range tests {
		if got := NormalizeDatabaseType(input); got != want {
			t.Fatalf("NormalizeDatabaseType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSupportedDatabaseTypes(t *testing.T) {
	if got := SupportedDatabaseTypes(); got != "sqlite, mysql" {
		t.Fatalf("SupportedDatabaseTypes() = %q, want %q", got, "sqlite, mysql")
	}
}
