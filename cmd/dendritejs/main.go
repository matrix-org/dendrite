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
	"net/http"
	"syscall/js"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/syncapi"
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
		cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)
	fed.Client = *gomatrixserverlib.NewClientWithTransport(tr)

	return fed
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
	cfg.SetDefaults()
	cfg.Kafka.UseNaffka = true
	cfg.Database.Account = "file:dendritejs_account.db"
	cfg.Database.AppService = "file:dendritejs_appservice.db"
	cfg.Database.Device = "file:dendritejs_device.db"
	cfg.Database.FederationSender = "file:dendritejs_fedsender.db"
	cfg.Database.MediaAPI = "file:dendritejs_mediaapi.db"
	cfg.Database.Naffka = "file:dendritejs_naffka.db"
	cfg.Database.PublicRoomsAPI = "file:dendritejs_publicrooms.db"
	cfg.Database.RoomServer = "file:dendritejs_roomserver.db"
	cfg.Database.ServerKey = "file:dendritejs_serverkey.db"
	cfg.Database.SyncAPI = "file:dendritejs_syncapi.db"
	cfg.Kafka.Topics.UserUpdates = "user_updates"
	cfg.Kafka.Topics.OutputTypingEvent = "output_typing_event"
	cfg.Kafka.Topics.OutputClientData = "output_client_data"
	cfg.Kafka.Topics.OutputRoomEvent = "output_room_event"
	cfg.Matrix.TrustedIDServers = []string{
		"matrix.org", "vector.im",
	}
	cfg.Matrix.KeyID = libp2pMatrixKeyID
	cfg.Matrix.PrivateKey = generateKey()

	serverName, node := createP2PNode(cfg.Matrix.PrivateKey)
	cfg.Matrix.ServerName = gomatrixserverlib.ServerName(serverName)

	if err := cfg.Derive(); err != nil {
		logrus.Fatalf("Failed to derive values from config: %s", err)
	}
	base := basecomponent.NewBaseDendrite(cfg, "Monolith", false)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := createFederationClient(cfg, node)
	keyRing := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			&libp2pKeyFetcher{},
		},
		KeyDatabase: keyDB,
	}
	p2pPublicRoomProvider := NewLibP2PPublicRoomsProvider(node)

	rsAPI := roomserver.SetupRoomServerComponent(base, keyRing, federation)
	eduInputAPI := eduserver.SetupEDUServerComponent(base, cache.New())
	asQuery := appservice.SetupAppServiceAPIComponent(
		base, accountDB, deviceDB, federation, rsAPI, transactions.New(),
	)
	fedSenderAPI := federationsender.SetupFederationSenderComponent(base, federation, rsAPI, &keyRing)
	rsAPI.SetFederationSenderAPI(fedSenderAPI)

	clientapi.SetupClientAPIComponent(
		base, deviceDB, accountDB,
		federation, &keyRing, rsAPI,
		eduInputAPI, asQuery, transactions.New(), fedSenderAPI,
	)
	eduProducer := producers.NewEDUServerProducer(eduInputAPI)
	federationapi.SetupFederationAPIComponent(base, accountDB, deviceDB, federation, &keyRing, rsAPI, asQuery, fedSenderAPI, eduProducer)
	mediaapi.SetupMediaAPIComponent(base, deviceDB)
	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(string(base.Cfg.Database.PublicRoomsAPI))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}
	publicroomsapi.SetupPublicRoomsAPIComponent(base, deviceDB, publicRoomsDB, rsAPI, federation, p2pPublicRoomProvider)
	syncapi.SetupSyncAPIComponent(base, deviceDB, accountDB, rsAPI, federation, cfg)

	httpHandler := common.WrapHandlerInCORS(base.APIMux)

	http.Handle("/", httpHandler)

	// Expose the matrix APIs via libp2p-js - for federation traffic
	if node != nil {
		go func() {
			logrus.Info("Listening on libp2p-js host ID ", node.Id)
			s := JSServer{
				Mux: http.DefaultServeMux,
			}
			s.ListenAndServe("p2p")
		}()
	}

	// Expose the matrix APIs via fetch - for local traffic
	go func() {
		logrus.Info("Listening for service-worker fetch traffic")
		s := JSServer{
			Mux: http.DefaultServeMux,
		}
		s.ListenAndServe("fetch")
	}()

	// We want to block forever to let the fetch and libp2p handler serve the APIs
	select {}
}
