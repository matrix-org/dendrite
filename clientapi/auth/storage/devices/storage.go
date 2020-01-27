package devices

import (
	"context"
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices/postgres"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*authtypes.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*authtypes.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]authtypes.Device, error)
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string) (dev *authtypes.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveAllDevices(ctx context.Context, localpart string) error
}

func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewDatabase(dataSourceName, serverName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName, serverName)
	case "file":
		return sqlite3.NewDatabase(dataSourceName, serverName)
	default:
		return postgres.NewDatabase(dataSourceName, serverName)
	}
}
