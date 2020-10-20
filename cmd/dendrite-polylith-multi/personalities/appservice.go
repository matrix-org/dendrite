package personalities

import (
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
)

func Appservice(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	rsAPI := base.RoomserverHTTPClient()

	intAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
	appservice.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(
		base.Cfg.AppServiceAPI.InternalAPI.Listen, // internal listener
		setup.NoListener, // external listener
		nil, nil,
	)
}
