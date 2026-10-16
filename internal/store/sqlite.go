package store

import (
	"database/sql"
	modernsqlite "modernc.org/sqlite"
)

func init() {
	// whatsmeow's sqlstore expects the SQLite dialect/driver name "sqlite3".
	// Register the pure-Go modernc driver under that compatibility name too.
	sql.Register("sqlite3", &modernsqlite.Driver{})
}
