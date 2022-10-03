package streams

import (
	"context"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type Streams struct {
	PDUStreamProvider              StreamProvider
	TypingStreamProvider           StreamProvider
	ReceiptStreamProvider          StreamProvider
	InviteStreamProvider           StreamProvider
	SendToDeviceStreamProvider     StreamProvider
	AccountDataStreamProvider      StreamProvider
	DeviceListStreamProvider       StreamProvider
	NotificationDataStreamProvider StreamProvider
	PresenceStreamProvider         StreamProvider
}

func NewSyncStreamProviders(
	d storage.Database, userAPI userapi.SyncUserAPI,
	rsAPI rsapi.SyncRoomserverAPI, keyAPI keyapi.SyncKeyAPI,
	eduCache *caching.EDUCache, lazyLoadCache caching.LazyLoadCache, notifier *notifier.Notifier,
) *Streams {
	streams := &Streams{
		PDUStreamProvider: &PDUStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
			lazyLoadCache:         lazyLoadCache,
			rsAPI:                 rsAPI,
			notifier:              notifier,
		},
		TypingStreamProvider: &TypingStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
			EDUCache:              eduCache,
		},
		ReceiptStreamProvider: &ReceiptStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
		},
		InviteStreamProvider: &InviteStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
		},
		SendToDeviceStreamProvider: &SendToDeviceStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
		},
		AccountDataStreamProvider: &AccountDataStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
			userAPI:               userAPI,
		},
		NotificationDataStreamProvider: &NotificationDataStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
		},
		DeviceListStreamProvider: &DeviceListStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
			rsAPI:                 rsAPI,
			keyAPI:                keyAPI,
		},
		PresenceStreamProvider: &PresenceStreamProvider{
			DefaultStreamProvider: DefaultStreamProvider{DB: d},
			notifier:              notifier,
		},
	}

	ctx := context.TODO()
	snapshot, err := d.NewDatabaseSnapshot(ctx)
	if err != nil {
		panic(err)
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	streams.PDUStreamProvider.Setup(ctx, snapshot)
	streams.TypingStreamProvider.Setup(ctx, snapshot)
	streams.ReceiptStreamProvider.Setup(ctx, snapshot)
	streams.InviteStreamProvider.Setup(ctx, snapshot)
	streams.SendToDeviceStreamProvider.Setup(ctx, snapshot)
	streams.AccountDataStreamProvider.Setup(ctx, snapshot)
	streams.NotificationDataStreamProvider.Setup(ctx, snapshot)
	streams.DeviceListStreamProvider.Setup(ctx, snapshot)
	streams.PresenceStreamProvider.Setup(ctx, snapshot)

	succeeded = true
	return streams
}

func (s *Streams) Latest(ctx context.Context) types.StreamingToken {
	return types.StreamingToken{
		PDUPosition:              s.PDUStreamProvider.LatestPosition(ctx),
		TypingPosition:           s.TypingStreamProvider.LatestPosition(ctx),
		ReceiptPosition:          s.ReceiptStreamProvider.LatestPosition(ctx),
		InvitePosition:           s.InviteStreamProvider.LatestPosition(ctx),
		SendToDevicePosition:     s.SendToDeviceStreamProvider.LatestPosition(ctx),
		AccountDataPosition:      s.AccountDataStreamProvider.LatestPosition(ctx),
		NotificationDataPosition: s.NotificationDataStreamProvider.LatestPosition(ctx),
		DeviceListPosition:       s.DeviceListStreamProvider.LatestPosition(ctx),
		PresencePosition:         s.PresenceStreamProvider.LatestPosition(ctx),
	}
}
