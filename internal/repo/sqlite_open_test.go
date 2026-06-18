package repo

import (
	"path/filepath"
	"testing"
)

func TestOpenSQLiteRegistersDriver(t *testing.T) {
	db, err := OpenSQLite(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("open sqlite through repo helper: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping sqlite: %v", err)
	}
}
