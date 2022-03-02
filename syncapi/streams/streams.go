package streams

import (
	"context"

	"github.com/matrix-org/dendrite/eduserver/cache"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type Streams struct {
	PDUStreamProvider          types.StreamProvider
	TypingStreamProvider       types.StreamProvider
	ReceiptStreamProvider      types.StreamProvider
	InviteStreamProvider       types.StreamProvider
	SendToDeviceStreamProvider types.StreamProvider
	AccountDataStreamProvider  types.StreamProvider
	DeviceListStreamProvider   types.StreamProvider
}

func NewSyncStreamProviders(
	d storage.Database, userAPI userapi.UserInternalAPI,
	rsAPI rsapi.RoomserverInternalAPI, keyAPI keyapi.KeyInternalAPI,
	eduCache *cache.EDUCache,
) *Streams {
	streams := &Streams{
		PDUStreamProvider: &PDUStreamProvider{
			StreamProvider: StreamProvider{DB: d},
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
		DeviceListStreamProvider: &DeviceListStreamProvider{
			StreamProvider: StreamProvider{DB: d},
			rsAPI:          rsAPI,
			keyAPI:         keyAPI,
		},
	}

	streams.PDUStreamProvider.Setup()
	streams.TypingStreamProvider.Setup()
	streams.ReceiptStreamProvider.Setup()
	streams.InviteStreamProvider.Setup()
	streams.SendToDeviceStreamProvider.Setup()
	streams.AccountDataStreamProvider.Setup()
	streams.DeviceListStreamProvider.Setup()

	return streams
}

func (s *Streams) Latest(ctx context.Context) types.StreamingToken {
	return types.StreamingToken{
		PDUPosition:          s.PDUStreamProvider.LatestPosition(ctx),
		TypingPosition:       s.TypingStreamProvider.LatestPosition(ctx),
		ReceiptPosition:      s.ReceiptStreamProvider.LatestPosition(ctx),
		InvitePosition:       s.InviteStreamProvider.LatestPosition(ctx),
		SendToDevicePosition: s.SendToDeviceStreamProvider.LatestPosition(ctx),
		AccountDataPosition:  s.AccountDataStreamProvider.LatestPosition(ctx),
		DeviceListPosition:   s.DeviceListStreamProvider.LatestPosition(ctx),
	}
}
