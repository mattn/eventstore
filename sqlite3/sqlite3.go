package sqlite3

import (
	"github.com/jmoiron/sqlx"
)

type SQLite3Backend struct {
	*sqlx.DB
	DatabaseURL  string
	MaxOpenConns int
	MaxIdleConns int
}

func (b *SQLite3Backend) Close() {
	b.DB.Close()
}
