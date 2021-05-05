package internal

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
)

// PushserverInternalAPI implements api.PushserverInternalAPI
type PushserverInternalAPI struct {
	DB  storage.Database
	Cfg *config.PushServer
}

func NewPushserverAPI(
	cfg *config.PushServer, pushserverDB storage.Database,
) *PushserverInternalAPI {
	a := &PushserverInternalAPI{
		DB:  pushserverDB,
		Cfg: cfg,
	}
	return a
}

// SetRoomAlias implements RoomserverAliasAPI
func (p *PushserverInternalAPI) QueryExample(
	ctx context.Context,
	request *api.QueryExampleRequest,
	response *api.QueryExampleResponse,
) error {
	// Implement QueryExample here!

	return nil
}
