package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

const registrationTokensSchema = `
CREATE TABLE IF NOT EXISTS roomserver_registration_tokens (
	token TEXT PRIMARY KEY,
	pending BIGINT,
	completed BIGINT,
	uses_allowed BIGINT,
	expiry_time BIGINT
);
`

func CreateRegistrationTokensTable(db *sql.DB) error {
	_, err := db.Exec(registrationTokensSchema)
	return err
}

func RegistrationTokenExists(ctx context.Context, tx *sql.Tx, token string) (bool, error) {
	fmt.Println("here!!")
	return true, nil
}
