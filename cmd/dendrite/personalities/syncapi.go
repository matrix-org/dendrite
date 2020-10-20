package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/syncapi"
)

func SyncAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	federation := base.CreateFederationClient()

	rsAPI := base.RoomserverHTTPClient()

	syncapi.AddPublicRoutes(
		base.PublicClientAPIMux, userAPI, rsAPI,
		base.KeyServerHTTPClient(),
		federation, &cfg.SyncAPI,
	)

	base.SetupAndServeHTTP(
		base.Cfg.SyncAPI.InternalAPI.Listen,
		base.Cfg.SyncAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
