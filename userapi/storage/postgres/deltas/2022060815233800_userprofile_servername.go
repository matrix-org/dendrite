package deltas

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

var serverName gomatrixserverlib.ServerName

func LoadProfilePrimaryKey(m *sqlutil.Migrations, s gomatrixserverlib.ServerName) {
	serverName = s
	m.AddMigration(UpProfilePrimaryKey, DownProfilePrimaryKey)
}

func UpProfilePrimaryKey(tx *sql.Tx) error {
	_, err := tx.Exec(fmt.Sprintf(`ALTER TABLE account_profiles ADD COLUMN IF NOT EXISTS servername TEXT NOT NULL DEFAULT '%s';
		ALTER TABLE account_profiles DROP CONSTRAINT account_profiles_pkey;
		ALTER TABLE account_profiles ADD PRIMARY KEY (localpart, servername);
		ALTER TABLE account_profiles ALTER COLUMN servername DROP DEFAULT;`, serverName))
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownProfilePrimaryKey(tx *sql.Tx) error {
	_, err := tx.Exec(`ALTER TABLE account_profiles DROP COLUMN IF EXISTS servername;
		ALTER TABLE account_profiles ADD PRIMARY KEY(localpart);`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
