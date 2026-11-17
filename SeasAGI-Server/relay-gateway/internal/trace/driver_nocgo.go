//go:build !cgo

package trace

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func openSQLite(dbPath string) (*sql.DB, error) {
	// WAL + synchronous=NORMAL：批量提交时不再每事务 fsync，兼顾吞吐与崩溃一致性
	// （应用崩溃不丢已提交数据，仅极端掉电可能丢最后一批）。
	return sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
}
