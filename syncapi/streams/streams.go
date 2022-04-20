package streams

import (
	"context"

	"github.com/matrix-org/dendrite/internal/caching"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type Streams struct {
	PDUStreamProvider              types.StreamProvider
	TypingStreamProvider           types.StreamProvider
	ReceiptStreamProvider          types.StreamProvider
	InviteStreamProvider           types.StreamProvider
	SendToDeviceStreamProvider     types.StreamProvider
	AccountDataStreamProvider      types.StreamProvider
	DeviceListStreamProvider       types.StreamProvider
	NotificationDataStreamProvider types.StreamProvider
	PresenceStreamProvider         types.StreamProvider
}

func NewSyncStreamProviders(
	d storage.Database, userAPI userapi.UserInternalAPI,
	rsAPI rsapi.RoomserverInternalAPI, keyAPI keyapi.KeyInternalAPI,
	eduCache *caching.EDUCache, lazyLoadCache *caching.LazyLoadCache, notifier *notifier.Notifier,
) *Streams {
	streams := &Streams{
		PDUStreamProvider: &PDUStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			lazyLoadCache:  lazyLoadCache,
		},
		TypingStreamProvider: &TypingStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			EDUCache:       eduCache,
		},
		ReceiptStreamProvider: &ReceiptStreamProvider{
			StreamProvider: StreamProvider{DB: d},
		},
		InviteStreamProvider: &InviteStreamProvider{
			StreamProvider: StreamProvider{DB: d},
		},
		SendToDeviceStreamProvider: &SendToDeviceStreamProvider{
			StreamProvider: StreamProvider{DB: d},
		},
		AccountDataStreamProvider: &AccountDataStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			userAPI:        userAPI,
		},
		NotificationDataStreamProvider: &NotificationDataStreamProvider{
			StreamProvider: StreamProvider{DB: d},
		},
		DeviceListStreamProvider: &DeviceListStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			rsAPI:          rsAPI,
			keyAPI:         keyAPI,
		},
		PresenceStreamProvider: &PresenceStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			notifier:       notifier,
		},
	}

	streams.PDUStreamProvider.Setup()
	streams.TypingStreamProvider.Setup()
	streams.ReceiptStreamProvider.Setup()
	streams.InviteStreamProvider.Setup()
	streams.SendToDeviceStreamProvider.Setup()
	streams.AccountDataStreamProvider.Setup()
	streams.NotificationDataStreamProvider.Setup()
	streams.DeviceListStreamProvider.Setup()
	streams.PresenceStreamProvider.Setup()

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
