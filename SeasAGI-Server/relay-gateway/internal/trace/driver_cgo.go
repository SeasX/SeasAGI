//go:build cgo

package trace

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func openSQLite(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
}
