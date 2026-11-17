//go:build cgo

package trace

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func openSQLite(dbPath string) (*sql.DB, error) {
	// WAL + synchronous=NORMAL：与 !cgo 实现保持一致，批量写入时避免逐事务 fsync。
	return sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
}
