package authorization

import (
	"github.com/matrix-org/dendrite/authorization"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/zion"
	log "github.com/sirupsen/logrus"
)

func NewAuthorization(cfg *config.ClientAPI) authorization.Authorization {
	// Load authorization manager for Zion
	if cfg.PublicKeyAuthentication.Ethereum.EnableAuthz {
		auth, err := zion.NewZionAuthorization()

		if err != nil {
			log.Errorln("Failed to initialise Zion authorization manager. Using default.", err)
		} else {
			return auth
		}
	}

	return &authorization.DefaultAuthorization{}
}
