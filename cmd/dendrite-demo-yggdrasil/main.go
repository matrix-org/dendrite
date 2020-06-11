// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/convert"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/signing"
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
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/sirupsen/logrus"
)

var (
	instanceName = flag.String("name", "dendrite-p2p-ygg", "the name of this P2P demo instance")
	instancePort = flag.Int("port", 8008, "the port that the client API will listen on")
	instancePeer = flag.String("peer", "", "an internet Yggdrasil peer to connect to")
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
		raw, err := hex.DecodeString(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("hex.DecodeString: %w", err)
		}
		converted := convert.Ed25519PublicKeyToCurve25519(ed25519.PublicKey(raw))
		convhex := hex.EncodeToString(converted)
		return n.Dial("curve25519", convhex)
	}
	yggdialerctx := func(ctx context.Context, network, address string) (net.Conn, error) {
		return yggdialer(network, address)
	}
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				ResponseHeaderTimeout: 15 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				DialContext:           yggdialerctx,
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
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	ygg, err := yggconn.Setup(*instanceName, *instancePeer)
	if err != nil {
		panic(err)
	}

	cfg := &config.Dendrite{}
	cfg.SetDefaults()
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(ygg.DerivedServerName())
	cfg.Matrix.PrivateKey = ygg.SigningPrivateKey()
	cfg.Matrix.KeyID = gomatrixserverlib.KeyID(signing.KeyID)
	cfg.Kafka.UseNaffka = true
	cfg.Kafka.Topics.OutputRoomEvent = "roomserverOutput"
	cfg.Kafka.Topics.OutputClientData = "clientapiOutput"
	cfg.Kafka.Topics.OutputTypingEvent = "typingServerOutput"
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

	serverKeyAPI := &signing.YggdrasilKeys{}
	keyRing := serverKeyAPI.KeyRing()

	rsComponent := roomserver.NewInternalAPI(
		base, keyRing, federation,
	)
	rsAPI := rsComponent

	eduInputAPI := eduserver.NewInternalAPI(
		base, cache.New(), deviceDB,
	)

	asAPI := appservice.NewInternalAPI(base, accountDB, deviceDB, rsAPI)

	fsAPI := federationsender.NewInternalAPI(
		base, federation, rsAPI, keyRing,
	)

	rsComponent.SetFederationSenderAPI(fsAPI)

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
		FederationSenderAPI: fsAPI,
		RoomserverAPI:       rsAPI,
		//ServerKeyAPI:        serverKeyAPI,

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
		logrus.Info("Listening on ", ygg.DerivedServerName())
		logrus.Fatal(httpServer.Serve(ygg))
	}()
	go func() {
		httpBindAddr := fmt.Sprintf(":%d", *instancePort)
		logrus.Info("Listening on ", httpBindAddr)
		logrus.Fatal(http.ListenAndServe(httpBindAddr, nil))
	}()

	select {}
}
