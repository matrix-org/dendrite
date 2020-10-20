package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/mediaapi"
)

func MediaAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	client := base.CreateClient()

	mediaapi.AddPublicRoutes(base.PublicMediaAPIMux, &base.Cfg.MediaAPI, userAPI, client)

	base.SetupAndServeHTTP(
		base.Cfg.MediaAPI.InternalAPI.Listen,
		base.Cfg.MediaAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
