// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package monolith

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"

	pineconeConnections "github.com/matrix-org/pinecone/connections"
	pineconeMulticast "github.com/matrix-org/pinecone/multicast"
	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeEvents "github.com/matrix-org/pinecone/router/events"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

type P2PMonolith struct {
	Sessions     *pineconeSessions.Sessions
	Multicast    *pineconeMulticast.Multicast
	ConnManager  *pineconeConnections.ConnectionManager
	Router       *pineconeRouter.Router
	EventChannel chan pineconeEvents.Event
}

func GenerateDefaultConfig(sk ed25519.PrivateKey, storageDir string, dbPrefix string) *config.Dendrite {
	cfg := config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{
		Generate:   true,
		Monolithic: true,
	})
	cfg.Global.PrivateKey = sk
	cfg.Global.JetStream.StoragePath = config.Path(fmt.Sprintf("%s/", filepath.Join(storageDir, dbPrefix)))
	cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-account.db", filepath.Join(storageDir, dbPrefix)))
	cfg.MediaAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.SyncAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.RoomServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", filepath.Join(storageDir, dbPrefix)))
	cfg.KeyServer.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-keyserver.db", filepath.Join(storageDir, dbPrefix)))
	cfg.FederationAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", filepath.Join(storageDir, dbPrefix)))
	cfg.RelayAPI.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-relayapi.db", filepath.Join(storageDir, dbPrefix)))
	cfg.MSCs.MSCs = []string{"msc2836", "msc2946"}
	cfg.MSCs.Database.ConnectionString = config.DataSource(fmt.Sprintf("file:%s-mscs.db", filepath.Join(storageDir, dbPrefix)))
	cfg.ClientAPI.RegistrationDisabled = false
	cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = true
	cfg.MediaAPI.BasePath = config.Path(filepath.Join(storageDir, "media"))
	cfg.MediaAPI.AbsBasePath = config.Path(filepath.Join(storageDir, "media"))
	cfg.SyncAPI.Fulltext.Enabled = true
	cfg.SyncAPI.Fulltext.IndexPath = config.Path(filepath.Join(storageDir, "search"))
	if err := cfg.Derive(); err != nil {
		panic(err)
	}

	return &cfg
}

func (p *P2PMonolith) SetupPinecone(sk ed25519.PrivateKey) {
	p.EventChannel = make(chan pineconeEvents.Event)
	p.Router = pineconeRouter.NewRouter(logrus.WithField("pinecone", "router"), sk)
	p.Router.EnableHopLimiting()
	p.Router.EnableWakeupBroadcasts()
	p.Router.Subscribe(p.EventChannel)

	p.Sessions = pineconeSessions.NewSessions(logrus.WithField("pinecone", "sessions"), p.Router, []string{"matrix"})
	p.Multicast = pineconeMulticast.NewMulticast(logrus.WithField("pinecone", "multicast"), p.Router)
	p.ConnManager = pineconeConnections.NewConnectionManager(p.Router, nil)
}
