package threepid

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/threepid/postgres"
	"github.com/matrix-org/dendrite/userapi/storage/threepid/sqlite3"
)

type Database interface {
	InsertSession(ctx context.Context, clientSecret, threepid, token, nextlink string, validatedAt int64, validated bool, sendAttempt int) (int64, error)
	GetSession(ctx context.Context, sid int64) (*api.Session, error)
	GetSessionByThreePidAndSecret(ctx context.Context, threePid, ClientSecret string) (*api.Session, error)
	UpdateSendAttemptNextLink(ctx context.Context, sid int64, nextLink string) error
	DeleteSession(ctx context.Context, sid int64) error
	ValidateSession(ctx context.Context, sid int64, validatedAt int64) error
}

// Open opens a database connection.
func Open(dbProperties *config.DatabaseOptions) (Database, error) {
	switch {
	case dbProperties.ConnectionString.IsSQLite():
		return sqlite3.Open(dbProperties)
	case dbProperties.ConnectionString.IsPostgres():
		return postgres.Open(dbProperties)
	default:
		return nil, fmt.Errorf("unexpected database type")
	}
}
