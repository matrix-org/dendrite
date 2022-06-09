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
	_, err := tx.Exec(fmt.Sprintf(`
    ALTER TABLE account_profiles RENAME TO account_profiles_tmp;
	CREATE TABLE IF NOT EXISTS account_profiles (
    	localpart TEXT NOT NULL,
    	servername TEXT NOT NULL,
    	display_name TEXT,
    	avatar_url TEXT,
    	PRIMARY KEY (localpart, servername)
	);
    INSERT
    INTO account_profiles (
        localpart, servername, display_name, avatar_url
    )  SELECT
           localpart, '%s', display_name, avatar_url
    FROM account_profiles_tmp;
    DROP TABLE account_profiles_tmp;`, serverName))
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func DownProfilePrimaryKey(tx *sql.Tx) error {
	_, err := tx.Exec(`
    ALTER TABLE account_profiles RENAME TO account_profiles_tmp;
	CREATE TABLE IF NOT EXISTS account_profiles (
    	localpart TEXT NOT NULL PRIMARY KEY,
    	display_name TEXT,
    	avatar_url TEXT
	);
    INSERT
    INTO account_profiles (
        localpart, display_name, avatar_url
    )  SELECT
           localpart, display_name, avatar_url
    FROM account_profiles_tmp;
    DROP TABLE account_profiles_tmp;`)
	if err != nil {
		return fmt.Errorf("failed to execute downgrade: %w", err)
	}
	return nil
}
