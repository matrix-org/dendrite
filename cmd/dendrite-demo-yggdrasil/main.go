// Copyright 2017 Vector Creations Ltd
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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/serverkeyapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/sirupsen/logrus"
)

var (
	instanceName = flag.String("name", "dendrite-p2p", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8080, "the port that the client API will listen on")
)

type yggroundtripper struct {
	inner *http.Transport
}

func (y *yggroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func createFederationClient(
	base *basecomponent.BaseDendrite, n *yggconn.Node,
) *gomatrixserverlib.FederationClient {
	yggdialer := func(_, address string) (net.Conn, error) {
		tokens := strings.Split(address, ":")
		return n.Dial("curve25519", tokens[0])
	}
	yggdialerctx := func(ctx context.Context, network, address string) (net.Conn, error) {
		return yggdialer(network, address)
	}
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				DialContext: yggdialerctx,
			},
		},
	)
	return gomatrixserverlib.NewFederationClientWithTransport(
		base.Cfg.Matrix.ServerName, base.Cfg.Matrix.KeyID, base.Cfg.Matrix.PrivateKey, tr,
	)
}

// nolint:gocyclo
func main() {
	flag.Parse()

	// Build both ends of a HTTP multiplex.
	httpServer := &http.Server{
		Addr:         ":0",
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	ygg, err := yggconn.Setup(*instanceName)
	if err != nil {
		panic(err)
	}

	cfg := &config.Dendrite{}
	cfg.SetDefaults()
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(ygg.EncryptionPublicKey())
	cfg.Matrix.PrivateKey = ygg.SigningPrivateKey()
	cfg.Matrix.KeyID = gomatrixserverlib.KeyID("ed25519:auto")
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
	cfg.Kafka.Topics.UserUpdates = "userUpdates"
	cfg.Database.Account = config.DataSource(fmt.Sprintf("file:%s-account.db", *instanceName))
	cfg.Database.Device = config.DataSource(fmt.Sprintf("file:%s-device.db", *instanceName))
	cfg.Database.MediaAPI = config.DataSource(fmt.Sprintf("file:%s-mediaapi.db", *instanceName))
	cfg.Database.SyncAPI = config.DataSource(fmt.Sprintf("file:%s-syncapi.db", *instanceName))
	cfg.Database.RoomServer = config.DataSource(fmt.Sprintf("file:%s-roomserver.db", *instanceName))
	cfg.Database.ServerKey = config.DataSource(fmt.Sprintf("file:%s-serverkey.db", *instanceName))
	cfg.Database.FederationSender = config.DataSource(fmt.Sprintf("file:%s-federationsender.db", *instanceName))
	cfg.Database.AppService = config.DataSource(fmt.Sprintf("file:%s-appservice.db", *instanceName))
	cfg.Database.PublicRoomsAPI = config.DataSource(fmt.Sprintf("file:%s-publicroomsa.db", *instanceName))
	cfg.Database.Naffka = config.DataSource(fmt.Sprintf("file:%s-naffka.db", *instanceName))
	if err = cfg.Derive(); err != nil {
		panic(err)
	}

	base := basecomponent.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := createFederationClient(base, ygg)

	serverKeyAPI := serverkeyapi.NewInternalAPI(
		base.Cfg, federation, base.Caches,
	)
	if base.UseHTTPAPIs {
		serverkeyapi.AddInternalRoutes(base.InternalAPIMux, serverKeyAPI, base.Caches)
		serverKeyAPI = base.ServerKeyAPIClient()
	}
	keyRing := serverKeyAPI.KeyRing()

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
	rsAPI := rsComponent
	if base.UseHTTPAPIs {
		roomserver.AddInternalRoutes(base.InternalAPIMux, rsAPI)
		rsAPI = base.RoomserverHTTPClient()
	}

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), deviceDB,
	)
	if base.UseHTTPAPIs {
		eduserver.AddInternalRoutes(base.InternalAPIMux, eduInputAPI)
		eduInputAPI = base.EDUServerClient()
	}

	asAPI := appservice.NewInternalAPI(base, accountDB, deviceDB, rsAPI)
	if base.UseHTTPAPIs {
		appservice.AddInternalRoutes(base.InternalAPIMux, asAPI)
		asAPI = base.AppserviceHTTPClient()
	}

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)
	if base.UseHTTPAPIs {
		federationsender.AddInternalRoutes(base.InternalAPIMux, fsAPI)
		fsAPI = base.FederationSenderHTTPClient()
	}
	rsComponent.SetFederationSenderAPI(fsAPI)

	eduProducer := producers.NewEDUServerProducer(eduInputAPI)
	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(string(base.Cfg.Database.PublicRoomsAPI), base.Cfg.DbProperties(), cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
		DeviceDB:      deviceDB,
		FedClient:     federation,
		KeyRing:       keyRing,
		KafkaConsumer: base.KafkaConsumer,
		KafkaProducer: base.KafkaProducer,

		AppserviceAPI:       asAPI,
		EDUInternalAPI:      eduInputAPI,
		EDUProducer:         eduProducer,
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		ServerKeyAPI:        serverKeyAPI,

		PublicRoomsDB: publicRoomsDB,
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	internal.SetupHTTPAPI(
		http.DefaultServeMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		cfg,
		base.UseHTTPAPIs,
	)

	go func() {
		logrus.Info("Listening on ", ygg.EncryptionPublicKey())
		logrus.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, nil))
	}()

	select {}
}
