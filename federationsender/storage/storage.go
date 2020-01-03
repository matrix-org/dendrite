package storage

import (
	"context"
	"errors"
	"net/url"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/storage/postgres"
	"github.com/matrix-org/dendrite/federationsender/types"
)

type Database interface {
	common.PartitionStorer
	UpdateRoom(ctx context.Context, roomID, oldEventID, newEventID string, addHosts []types.JoinedHost, removeHosts []string) (joinedHosts []types.JoinedHost, err error)
	GetJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return nil, err
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName)
	default:
		return nil, errors.New("unknown schema")
	}
}
