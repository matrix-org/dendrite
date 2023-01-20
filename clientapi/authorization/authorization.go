package authorization

import (
	"flag"

	"github.com/matrix-org/dendrite/authorization"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/zion"
	log "github.com/sirupsen/logrus"
)

func NewRoomserverAuthorization(cfg *config.ClientAPI, roomQueryAPI roomserver.QueryEventsAPI) authorization.Authorization {

	if flag.Lookup("test.v") == nil {
		// normal run
		// Load authorization manager for Zion
		auth, err := zion.NewZionAuthorization(cfg, roomQueryAPI)
		if err != nil {
			// Cannot proceed without an authorization manager
			log.Fatalf("failed to initialise Zion authorization manager, using default. Error: %v", err)
		}

		return auth
	} else {
		// run under go test
		return &authorization.DefaultAuthorization{}
	}

}
