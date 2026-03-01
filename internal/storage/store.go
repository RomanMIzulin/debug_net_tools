package storage

import (
    "database/sql"
    _ "modernc.org/sqlite"
    "github.com/Masterminds/squirrel"
)

type Store struct{
	db *sql.DB

}
