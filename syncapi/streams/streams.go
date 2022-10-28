package streams

import (
	"context"
	"fmt"

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

func ToToken(provider StreamProvider, position types.StreamPosition) types.StreamingToken {
	switch t := provider.(type) {
	case *PDUStreamProvider:
		return types.StreamingToken{PDUPosition: position}
	case *TypingStreamProvider:
		return types.StreamingToken{TypingPosition: position}
	case *ReceiptStreamProvider:
		return types.StreamingToken{ReceiptPosition: position}
	case *SendToDeviceStreamProvider:
		return types.StreamingToken{SendToDevicePosition: position}
	case *InviteStreamProvider:
		return types.StreamingToken{InvitePosition: position}
	case *AccountDataStreamProvider:
		return types.StreamingToken{AccountDataPosition: position}
	case *DeviceListStreamProvider:
		return types.StreamingToken{DeviceListPosition: position}
	case *NotificationDataStreamProvider:
		return types.StreamingToken{NotificationDataPosition: position}
	case *PresenceStreamProvider:
		return types.StreamingToken{PresencePosition: position}
	default:
		panic(fmt.Sprintf("unknown stream provider: %T", t))
	}
	return types.StreamingToken{}
}

func IncrementalPositions(provider StreamProvider, current, since types.StreamingToken) (types.StreamPosition, types.StreamPosition) {
	switch t := provider.(type) {
	case *PDUStreamProvider:
		return current.PDUPosition, since.PDUPosition
	case *TypingStreamProvider:
		return current.TypingPosition, since.TypingPosition
	case *ReceiptStreamProvider:
		return current.ReceiptPosition, since.ReceiptPosition
	case *SendToDeviceStreamProvider:
		return current.SendToDevicePosition, since.SendToDevicePosition
	case *InviteStreamProvider:
		return current.InvitePosition, since.InvitePosition
	case *AccountDataStreamProvider:
		return current.AccountDataPosition, since.AccountDataPosition
	case *DeviceListStreamProvider:
		return current.DeviceListPosition, since.DeviceListPosition
	case *NotificationDataStreamProvider:
		return current.NotificationDataPosition, since.NotificationDataPosition
	case *PresenceStreamProvider:
		return current.PresencePosition, since.PresencePosition
	default:
		panic(fmt.Sprintf("unknown stream provider: %T", t))
	}
	return 0, 0
}
