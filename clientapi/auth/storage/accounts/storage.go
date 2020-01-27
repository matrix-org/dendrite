package accounts

import (
	"context"
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/mediaapi/storage/postgres"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	common.PartitionStorer
	GetAccountByPassword(ctx context.Context, localpart, plaintextPassword string) (*authtypes.Account, error)
	GetProfileByLocalpart(ctx context.Context, localpart string) (*authtypes.Profile, error)
	SetAvatarURL(ctx context.Context, localpart string, avatarURL string) error
	SetDisplayName(ctx context.Context, localpart string, displayName string) error
	CreateAccount(ctx context.Context, localpart, plaintextPassword, appserviceID string) (*authtypes.Account, error)
	UpdateMemberships(ctx context.Context, eventsToAdd []gomatrixserverlib.Event, idsToRemove []string) error
	GetMembershipInRoomByLocalpart(ctx context.Context, localpart, roomID string) (authtypes.Membership, error)
	GetMembershipsByLocalpart(ctx context.Context, localpart string) (memberships []authtypes.Membership, err error)
	SaveAccountData(ctx context.Context, localpart, roomID, dataType, content string) error
	GetAccountData(ctx context.Context, localpart string) (global []gomatrixserverlib.ClientEvent, rooms map[string][]gomatrixserverlib.ClientEvent, err error)
	GetAccountDataByType(ctx context.Context, localpart, roomID, dataType string) (data *gomatrixserverlib.ClientEvent, err error)
	GetNewNumericLocalpart(ctx context.Context) (int64, error)
	SaveThreePIDAssociation(ctx context.Context, threepid, localpart, medium string) (err error)
	RemoveThreePIDAssociation(ctx context.Context, threepid string, medium string) (err error)
	GetLocalpartForThreePID(ctx context.Context, threepid string, medium string) (localpart string, err error)
	GetThreePIDsForLocalpart(ctx context.Context, localpart string) (threepids []authtypes.ThreePID, err error)
	GetFilter(ctx context.Context, localpart string, filterID string) (*gomatrixserverlib.Filter, error)
	PutFilter(ctx context.Context, localpart string, filter *gomatrixserverlib.Filter) (string, error)
	CheckAccountAvailability(ctx context.Context, localpart string) (bool, error)
	GetAccountByLocalpart(ctx context.Context, localpart string) (*authtypes.Account, error)
}

func Open(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.Open(dataSourceName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.Open(dataSourceName)
	case "file":
	//	return sqlite3.Open(dataSourceName)
	default:
		return postgres.Open(dataSourceName)
	}
}
