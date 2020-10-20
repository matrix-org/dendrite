package personalities

import (
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/userapi"
)

func UserAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	accountDB := base.CreateAccountsDB()

	userAPI := userapi.NewInternalAPI(accountDB, &cfg.UserAPI, cfg.Derived.ApplicationServices, base.KeyServerHTTPClient())

	userapi.AddInternalRoutes(base.InternalAPIMux, userAPI)

	base.SetupAndServeHTTP(
		base.Cfg.UserAPI.InternalAPI.Listen, // internal listener
		setup.NoListener,                    // external listener
		nil, nil,
	)
}
