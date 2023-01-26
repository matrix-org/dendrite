package keyserver

import (
	"context"
	"testing"

	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

type mockKeyserverRoomserverAPI struct {
	leftUsers []string
}

func (m *mockKeyserverRoomserverAPI) QueryLeftUsers(ctx context.Context, req *roomserver.QueryLeftUsersRequest, res *roomserver.QueryLeftUsersResponse) error {
	res.LeftUsers = m.leftUsers
	return nil
}

// Merely tests that we can create an internal keyserver API
func Test_NewInternalAPI(t *testing.T) {
	rsAPI := &mockKeyserverRoomserverAPI{}
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, closeBase := testrig.CreateBaseDendrite(t, dbType)
		defer closeBase()
		_ = NewInternalAPI(base, &base.Cfg.KeyServer, nil, rsAPI)
	})
}
