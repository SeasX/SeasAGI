//go:build !cgo

package channel

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func openSQLite(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
}
