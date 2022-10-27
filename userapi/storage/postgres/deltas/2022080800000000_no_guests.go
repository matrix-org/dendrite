package deltas

import (
	"context"
	"database/sql"
	"fmt"
)

func UpNoGuests(ctx context.Context, tx *sql.Tx) error {
	// AddAccountType introduced a bug where each user that had was registered as a regular user, but without user_id, became a guest.
	_, err := tx.ExecContext(ctx, "UPDATE userapi_accounts SET account_type = 1 WHERE account_type = 2;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownNoGuests(ctx context.Context, tx *sql.Tx) error {
	return nil
}
