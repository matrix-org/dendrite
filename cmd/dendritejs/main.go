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

// +build wasm

package main

import (
	"crypto/ed25519"
	"fmt"
	"syscall/js"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/currentstateserver"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/userapi"
	go_http_js_libp2p "github.com/matrix-org/go-http-js-libp2p"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/sirupsen/logrus"

	_ "github.com/matrix-org/go-sqlite3-js"
)

var GitCommit string

func init() {
	fmt.Printf("[%s] dendrite.js starting...\n", GitCommit)
}

const keyNameEd25519 = "_go_ed25519_key"

func readKeyFromLocalStorage() (key ed25519.PrivateKey, err error) {
	localforage := js.Global().Get("localforage")
	if !localforage.Truthy() {
		err = fmt.Errorf("readKeyFromLocalStorage: no localforage")
		return
	}
	// https://localforage.github.io/localForage/
	item, ok := await(localforage.Call("getItem", keyNameEd25519))
	if !ok || !item.Truthy() {
		err = fmt.Errorf("readKeyFromLocalStorage: no key in localforage")
		return
	}
	fmt.Println("Found key in localforage")
	// extract []byte and make an ed25519 key
	seed := make([]byte, 32, 32)
	js.CopyBytesToGo(seed, item)

	return ed25519.NewKeyFromSeed(seed), nil
}

func writeKeyToLocalStorage(key ed25519.PrivateKey) error {
	localforage := js.Global().Get("localforage")
	if !localforage.Truthy() {
		return fmt.Errorf("writeKeyToLocalStorage: no localforage")
	}

	// make a Uint8Array from the key's seed
	seed := key.Seed()
	jsSeed := js.Global().Get("Uint8Array").New(len(seed))
	js.CopyBytesToJS(jsSeed, seed)
	// write it
	localforage.Call("setItem", keyNameEd25519, jsSeed)
	return nil
}

// taken from https://go-review.googlesource.com/c/go/+/150917

// await waits until the promise v has been resolved or rejected and returns the promise's result value.
// The boolean value ok is true if the promise has been resolved, false if it has been rejected.
// If v is not a promise, v itself is returned as the value and ok is true.
func await(v js.Value) (result js.Value, ok bool) {
	if v.Type() != js.TypeObject || v.Get("then").Type() != js.TypeFunction {
		return v, true
	}
	done := make(chan struct{})
	onResolve := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		result = args[0]
		ok = true
		close(done)
		return nil
	})
	defer onResolve.Release()
	onReject := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		result = args[0]
		ok = false
		close(done)
		return nil
	})
	defer onReject.Release()
	v.Call("then", onResolve, onReject)
	<-done
	return
}

func generateKey() ed25519.PrivateKey {
	// attempt to look for a seed in JS-land and if it exists use it.
	priv, err := readKeyFromLocalStorage()
	if err == nil {
		fmt.Println("Read key from localStorage")
		return priv
	}
	// generate a new key
	fmt.Println(err, " : Generating new ed25519 key")
	_, priv, err = ed25519.GenerateKey(nil)
	if err != nil {
		logrus.Fatalf("Failed to generate ed25519 key: %s", err)
	}
	if err := writeKeyToLocalStorage(priv); err != nil {
		fmt.Println("failed to write key to localStorage: ", err)
		// non-fatal, we'll just have amnesia for a while
	}
	return priv
}

func createFederationClient(cfg *config.Dendrite, node *go_http_js_libp2p.P2pLocalNode) *gomatrixserverlib.FederationClient {
	fmt.Println("Running in js-libp2p federation mode")
	fmt.Println("Warning: Federation with non-libp2p homeservers will not work in this mode yet!")
	tr := go_http_js_libp2p.NewP2pTransport(node)

	fed := gomatrixserverlib.NewFederationClient(
		cfg.Global.ServerName, cfg.Global.KeyID, cfg.Global.PrivateKey, true,
	)
	fed.Client = *gomatrixserverlib.NewClientWithTransport(true, tr)

	return fed
}

func createClient(node *go_http_js_libp2p.P2pLocalNode) *gomatrixserverlib.Client {
	tr := go_http_js_libp2p.NewP2pTransport(node)
	return gomatrixserverlib.NewClientWithTransport(true, tr)
}

func createP2PNode(privKey ed25519.PrivateKey) (serverName string, node *go_http_js_libp2p.P2pLocalNode) {
	hosted := "/dns4/rendezvous.matrix.org/tcp/8443/wss/p2p-websocket-star/"
	node = go_http_js_libp2p.NewP2pLocalNode("org.matrix.p2p.experiment", privKey.Seed(), []string{hosted}, "p2p")
	serverName = node.Id
	fmt.Println("p2p assigned ServerName: ", serverName)
	return
}

func main() {
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.UserAPI.AccountDatabase.ConnectionString = "file:/idb/dendritejs_account.db"
	cfg.AppServiceAPI.Database.ConnectionString = "file:/idb/dendritejs_appservice.db"
	cfg.UserAPI.DeviceDatabase.ConnectionString = "file:/idb/dendritejs_device.db"
	cfg.FederationSender.Database.ConnectionString = "file:/idb/dendritejs_fedsender.db"
	cfg.MediaAPI.Database.ConnectionString = "file:/idb/dendritejs_mediaapi.db"
	cfg.RoomServer.Database.ConnectionString = "file:/idb/dendritejs_roomserver.db"
	cfg.ServerKeyAPI.Database.ConnectionString = "file:/idb/dendritejs_serverkey.db"
	cfg.SyncAPI.Database.ConnectionString = "file:/idb/dendritejs_syncapi.db"
	cfg.CurrentStateServer.Database.ConnectionString = "file:/idb/dendritejs_currentstate.db"
	cfg.KeyServer.Database.ConnectionString = "file:/idb/dendritejs_e2ekey.db"
	cfg.Global.Kafka.UseNaffka = true
	cfg.Global.Kafka.Database.ConnectionString = "file:/idb/dendritejs_naffka.db"
	cfg.Global.Kafka.Topics.OutputTypingEvent = "output_typing_event"
	cfg.Global.Kafka.Topics.OutputSendToDeviceEvent = "output_send_to_device_event"
	cfg.Global.Kafka.Topics.OutputClientData = "output_client_data"
	cfg.Global.Kafka.Topics.OutputRoomEvent = "output_room_event"
	cfg.Global.TrustedIDServers = []string{
		"matrix.org", "vector.im",
	}
	cfg.Global.KeyID = libp2pMatrixKeyID
	cfg.Global.PrivateKey = generateKey()

	serverName, node := createP2PNode(cfg.Global.PrivateKey)
	cfg.Global.ServerName = gomatrixserverlib.ServerName(serverName)

	if err := cfg.Derive(); err != nil {
		logrus.Fatalf("Failed to derive values from config: %s", err)
	}
	base := setup.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := createFederationClient(cfg, node)
	keyAPI := keyserver.NewInternalAPI(&base.Cfg.KeyServer, federation, base.KafkaProducer)
	userAPI := userapi.NewInternalAPI(accountDB, deviceDB, cfg.Global.ServerName, nil, keyAPI)
	keyAPI.SetUserAPI(userAPI)

	fetcher := &libp2pKeyFetcher{}
	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			fetcher,
		},
		KeyDatabase: fetcher,
	}

	stateAPI := currentstateserver.NewInternalAPI(&base.Cfg.CurrentStateServer, base.KafkaConsumer)
	rsAPI := roomserver.NewInternalAPI(base, keyRing, federation)
	eduInputAPI := eduserver.NewInternalAPI(base, cache.New(), userAPI)
	asQuery := appservice.NewInternalAPI(
		base, userAPI, rsAPI,
	)
	fedSenderAPI := federationsender.NewInternalAPI(base, federation, rsAPI, stateAPI, &keyRing)
	rsAPI.SetFederationSenderAPI(fedSenderAPI)
	p2pPublicRoomProvider := NewLibP2PPublicRoomsProvider(node, fedSenderAPI, federation)

	monolith := setup.Monolith{
		Config:        base.Cfg,
		AccountDB:     accountDB,
		DeviceDB:      deviceDB,
		Client:        createClient(node),
		FedClient:     federation,
		KeyRing:       &keyRing,
		KafkaConsumer: base.KafkaConsumer,
		KafkaProducer: base.KafkaProducer,

		AppserviceAPI:       asQuery,
		EDUInternalAPI:      eduInputAPI,
		FederationSenderAPI: fedSenderAPI,
		RoomserverAPI:       rsAPI,
		StateAPI:            stateAPI,
		UserAPI:             userAPI,
		KeyAPI:              keyAPI,
		//ServerKeyAPI:        serverKeyAPI,
		ExtPublicRoomsProvider: p2pPublicRoomProvider,
	}
	monolith.AddAllPublicRoutes(base.PublicAPIMux)

	httputil.SetupHTTPAPI(
		base.BaseMux,
		base.PublicAPIMux,
		base.InternalAPIMux,
		&cfg.Global,
		base.UseHTTPAPIs,
	)

	// Expose the matrix APIs via libp2p-js - for federation traffic
	if node != nil {
		go func() {
			logrus.Info("Listening on libp2p-js host ID ", node.Id)
			s := JSServer{
				Mux: base.BaseMux,
			}
			s.ListenAndServe("p2p")
		}()
	}

	// Expose the matrix APIs via fetch - for local traffic
	go func() {
		logrus.Info("Listening for service-worker fetch traffic")
		s := JSServer{
			Mux: base.BaseMux,
		}
		s.ListenAndServe("fetch")
	}()

	// We want to block forever to let the fetch and libp2p handler serve the APIs
	select {}
}
