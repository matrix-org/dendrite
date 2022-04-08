//go:build wasm
// +build wasm

package sqlutil

import (
	"database/sql"
	"fmt"
	"runtime"
	"sort"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/pressly/goose"
)

type Migrations struct {
}

func NewMigrations() *Migrations {
	return &Migrations{}
}

func (m *Migrations) AddMigration(up func(*sql.Tx) error, down func(*sql.Tx) error) {
}

func (m *Migrations) AddNamedMigration(filename string, up func(*sql.Tx) error, down func(*sql.Tx) error) {

}

func (m *Migrations) RunDeltas(db *sql.DB, props *config.DatabaseOptions) error {
	return nil
}

func (m *Migrations) collect(current, target int64) (goose.Migrations, error) {
	var migrations goose.Migrations
	return migrations, nil
}
