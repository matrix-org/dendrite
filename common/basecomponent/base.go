// Copyright 2017 New Vector Ltd
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

package basecomponent

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/ed25519"

	"github.com/libp2p/go-libp2p"
	circuit "github.com/libp2p/go-libp2p-circuit"
	"github.com/libp2p/go-libp2p-core/peer"
	crypto "github.com/libp2p/go-libp2p-crypto"
	routing "github.com/libp2p/go-libp2p-routing"

	host "github.com/libp2p/go-libp2p-host"
	p2phttp "github.com/libp2p/go-libp2p-http"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	p2pdisc "github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/naffka"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"

	"github.com/gorilla/mux"
	sarama "gopkg.in/Shopify/sarama.v1"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/common/config"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/sirupsen/logrus"
)

// BaseDendrite is a base for creating new instances of dendrite. It parses
// command line flags and config, and exposes methods for creating various
// resources. All errors are handled by logging then exiting, so all methods
// should only be used during start up.
// Must be closed when shutting down.
type BaseDendrite struct {
	componentName string
	tracerCloser  io.Closer

	// APIMux should be used to register new public matrix api endpoints
	APIMux        *mux.Router
	httpClient    *http.Client
	Cfg           *config.Dendrite
	KafkaConsumer sarama.Consumer
	KafkaProducer sarama.SyncProducer

	// Store our libp2p object so that we can make outgoing connections from it
	// later
	LibP2P        host.Host
	LibP2PContext context.Context
	LibP2PCancel  context.CancelFunc
	LibP2PDHT     *dht.IpfsDHT
	LibP2PPubsub  *pubsub.PubSub
}

// NewBaseDendrite creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the compontent running, e.g. "SyncAPI"
func NewBaseDendrite(cfg *config.Dendrite, componentName string) *BaseDendrite {
	common.SetupStdLogging()
	common.SetupHookLogging(cfg.Logging, componentName)

	closer, err := cfg.SetupTracing("Dendrite" + componentName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start opentracing")
	}

	kafkaConsumer, kafkaProducer := setupKafka(cfg)

	if cfg.Matrix.ServerName == "p2p" {
		ctx, cancel := context.WithCancel(context.Background())

		privKey, err := crypto.UnmarshalEd25519PrivateKey(cfg.Matrix.PrivateKey[:])
		if err != nil {
			panic(err)
		}

		//defaultIP6ListenAddr, _ := multiaddr.NewMultiaddr("/ip6/::/tcp/0")
		var libp2pdht *dht.IpfsDHT
		libp2p, err := libp2p.New(ctx,
			libp2p.Identity(privKey),
			libp2p.DefaultListenAddrs,
			//libp2p.ListenAddrs(defaultIP6ListenAddr),
			libp2p.DefaultTransports,
			libp2p.Routing(func(h host.Host) (r routing.PeerRouting, err error) {
				libp2pdht, err = dht.New(ctx, h)
				if err != nil {
					return nil, err
				}
				libp2pdht.Validator = LibP2PValidator{}
				r = libp2pdht
				return
			}),
			libp2p.EnableAutoRelay(),
			libp2p.EnableRelay(circuit.OptHop),
		)
		if err != nil {
			panic(err)
		}

		libp2ppubsub, err := pubsub.NewFloodSub(context.Background(), libp2p, []pubsub.Option{
			pubsub.WithMessageSigning(true),
		}...)
		if err != nil {
			panic(err)
		}

		fmt.Println("Our public key:", privKey.GetPublic())
		fmt.Println("Our node ID:", libp2p.ID())
		fmt.Println("Our addresses:", libp2p.Addrs())

		cfg.Matrix.ServerName = gomatrixserverlib.ServerName(libp2p.ID().String())

		return &BaseDendrite{
			componentName: componentName,
			tracerCloser:  closer,
			Cfg:           cfg,
			APIMux:        mux.NewRouter().UseEncodedPath(),
			KafkaConsumer: kafkaConsumer,
			KafkaProducer: kafkaProducer,
			LibP2P:        libp2p,
			LibP2PContext: ctx,
			LibP2PCancel:  cancel,
			LibP2PDHT:     libp2pdht,
			LibP2PPubsub:  libp2ppubsub,
		}
	} else {
		return &BaseDendrite{
			componentName: componentName,
			tracerCloser:  closer,
			Cfg:           cfg,
			APIMux:        mux.NewRouter().UseEncodedPath(),
			KafkaConsumer: kafkaConsumer,
			KafkaProducer: kafkaProducer,
		}
	}
}

// Close implements io.Closer
func (b *BaseDendrite) Close() error {
	return b.tracerCloser.Close()
}

// CreateHTTPAppServiceAPIs returns the QueryAPI for hitting the appservice
// component over HTTP.
func (b *BaseDendrite) CreateHTTPAppServiceAPIs() appserviceAPI.AppServiceQueryAPI {
	a, err := appserviceAPI.NewAppServiceQueryAPIHTTP(b.Cfg.AppServiceURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("CreateHTTPAppServiceAPIs failed")
	}
	return a
}

// CreateHTTPRoomserverAPIs returns the AliasAPI, InputAPI and QueryAPI for hitting
// the roomserver over HTTP.
func (b *BaseDendrite) CreateHTTPRoomserverAPIs() (
	roomserverAPI.RoomserverAliasAPI,
	roomserverAPI.RoomserverInputAPI,
	roomserverAPI.RoomserverQueryAPI,
) {

	alias, err := roomserverAPI.NewRoomserverAliasAPIHTTP(b.Cfg.RoomServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("NewRoomserverAliasAPIHTTP failed")
	}
	input, err := roomserverAPI.NewRoomserverInputAPIHTTP(b.Cfg.RoomServerURL(), b.httpClient)
	if err != nil {
		logrus.WithError(err).Panic("NewRoomserverInputAPIHTTP failed", b.httpClient)
	}
	query, err := roomserverAPI.NewRoomserverQueryAPIHTTP(b.Cfg.RoomServerURL(), nil)
	if err != nil {
		logrus.WithError(err).Panic("NewRoomserverQueryAPIHTTP failed", b.httpClient)
	}
	return alias, input, query
}

// CreateHTTPEDUServerAPIs returns eduInputAPI for hitting the EDU
// server over HTTP
func (b *BaseDendrite) CreateHTTPEDUServerAPIs() eduServerAPI.EDUServerInputAPI {
	e, err := eduServerAPI.NewEDUServerInputAPIHTTP(b.Cfg.EDUServerURL(), nil)
	if err != nil {
		logrus.WithError(err).Panic("NewEDUServerInputAPIHTTP failed", b.httpClient)
	}
	return e
}

// CreateHTTPFederationSenderAPIs returns FederationSenderQueryAPI for hitting
// the federation sender over HTTP
func (b *BaseDendrite) CreateHTTPFederationSenderAPIs() federationSenderAPI.FederationSenderQueryAPI {
	f, err := federationSenderAPI.NewFederationSenderQueryAPIHTTP(b.Cfg.FederationSenderURL(), nil)
	if err != nil {
		logrus.WithError(err).Panic("NewFederationSenderQueryAPIHTTP failed", b.httpClient)
	}
	return f
}

// CreateDeviceDB creates a new instance of the device database. Should only be
// called once per component.
func (b *BaseDendrite) CreateDeviceDB() devices.Database {
	db, err := devices.NewDatabase(string(b.Cfg.Database.Device), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to devices db")
	}

	return db
}

// CreateAccountsDB creates a new instance of the accounts database. Should only
// be called once per component.
func (b *BaseDendrite) CreateAccountsDB() accounts.Database {
	db, err := accounts.NewDatabase(string(b.Cfg.Database.Account), b.Cfg.Matrix.ServerName)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	return db
}

// CreateKeyDB creates a new instance of the key database. Should only be called
// once per component.
func (b *BaseDendrite) CreateKeyDB() keydb.Database {
	db, err := keydb.NewDatabase(
		string(b.Cfg.Database.ServerKey),
		b.Cfg.Matrix.ServerName,
		b.Cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey),
		b.Cfg.Matrix.KeyID,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to keys db")
	}
	if b.LibP2P != nil {
		mdns := mDNSListener{
			host:  b.LibP2P,
			keydb: db,
		}
		serv, err := p2pdisc.NewMdnsService(
			b.LibP2PContext,
			b.LibP2P,
			time.Second*10,
			"_matrix-dendrite-p2p._tcp",
		)
		if err != nil {
			panic(err)
		}
		serv.RegisterNotifee(&mdns)
	}

	return db
}

// CreateFederationClient creates a new federation client. Should only be called
// once per component.
func (b *BaseDendrite) CreateFederationClient() *gomatrixserverlib.FederationClient {
	if b.LibP2P != nil {
		fmt.Println("Running in libp2p federation mode")
		fmt.Println("Warning: Federation with non-libp2p homeservers will not work in this mode yet!")
		tr := &http.Transport{}
		tr.RegisterProtocol(
			"matrix",
			p2phttp.NewTransport(b.LibP2P, p2phttp.ProtocolOption("/matrix")),
		)
		return gomatrixserverlib.NewFederationClientWithTransport(
			b.Cfg.Matrix.ServerName, b.Cfg.Matrix.KeyID, b.Cfg.Matrix.PrivateKey, tr,
		)
	} else {
		fmt.Println("Running in regular federation mode")
		return gomatrixserverlib.NewFederationClient(
			b.Cfg.Matrix.ServerName, b.Cfg.Matrix.KeyID, b.Cfg.Matrix.PrivateKey,
		)
	}
}

// SetupAndServeHTTP sets up the HTTP server to serve endpoints registered on
// ApiMux under /api/ and adds a prometheus handler under /metrics.
func (b *BaseDendrite) SetupAndServeHTTP(bindaddr string, listenaddr string) {
	// If a separate bind address is defined, listen on that. Otherwise use
	// the listen address
	var addr string
	if bindaddr != "" {
		addr = bindaddr
	} else {
		addr = listenaddr
	}

	common.SetupHTTPAPI(http.DefaultServeMux, common.WrapHandlerInCORS(b.APIMux))
	logrus.Infof("Starting %s server on %s", b.componentName, addr)

	err := http.ListenAndServe(addr, nil)

	if err != nil {
		logrus.WithError(err).Fatal("failed to serve http")
	}

	logrus.Infof("Stopped %s server on %s", b.componentName, addr)
}

// setupKafka creates kafka consumer/producer pair from the config. Checks if
// should use naffka.
func setupKafka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	if cfg.Kafka.UseNaffka {
		var naffkaDB *naffka.DatabaseImpl
		uri, err := url.Parse(string(cfg.Database.Naffka))
		if err != nil {
			panic(err)
		}
		switch uri.Scheme {
		case "file":
			db, err := sql.Open(common.SQLiteDriverName(), string(cfg.Database.Naffka))
			if err != nil {
				logrus.WithError(err).Panic("Failed to open naffka database")
			}

			naffkaDB, err = naffka.NewSqliteDatabase(db)
			if err != nil {
				logrus.WithError(err).Panic("Failed to setup naffka database")
			}
		case "postgres":
			fallthrough
		default:
			db, err := sql.Open("postgres", string(cfg.Database.Naffka))
			if err != nil {
				logrus.WithError(err).Panic("Failed to open naffka database")
			}

			naffkaDB, err = naffka.NewPostgresqlDatabase(db)
			if err != nil {
				logrus.WithError(err).Panic("Failed to setup naffka database")
			}
		}

		naff, err := naffka.New(naffkaDB)
		if err != nil {
			panic(err)
		}

		//logrus.WithError(err).Panic("Failed to setup naffka")

		return naff, naff
	}

	consumer, err := sarama.NewConsumer(cfg.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to start kafka consumer")
	}

	producer, err := sarama.NewSyncProducer(cfg.Kafka.Addresses, nil)
	if err != nil {
		logrus.WithError(err).Panic("failed to setup kafka producers")
	}

	return consumer, producer
}

// setupNaffka creates kafka consumer/producer pair from the config.
func setupNaffka(cfg *config.Dendrite) (sarama.Consumer, sarama.SyncProducer) {
	var err error
	var db *sql.DB
	var naffkaDB *naffka.DatabaseImpl

	uri, err := url.Parse(string(cfg.Database.Naffka))
	if err != nil || uri.Scheme == "file" {
		db, err = sql.Open(common.SQLiteDriverName(), string(cfg.Database.Naffka))
		if err != nil {
			logrus.WithError(err).Panic("Failed to open naffka database")
		}

		naffkaDB, err = naffka.NewSqliteDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
	} else {
		db, err = sql.Open("postgres", string(cfg.Database.Naffka))
		if err != nil {
			logrus.WithError(err).Panic("Failed to open naffka database")
		}

		naffkaDB, err = naffka.NewPostgresqlDatabase(db)
		if err != nil {
			logrus.WithError(err).Panic("Failed to setup naffka database")
		}
	}

	if naffkaDB == nil {
		panic("naffka connection string not understood")
	}

	naff, err := naffka.New(naffkaDB)
	if err != nil {
		logrus.WithError(err).Panic("Failed to setup naffka")
	}

	return naff, naff
}

type mDNSListener struct {
	keydb keydb.Database
	host  host.Host
}

func (n *mDNSListener) HandlePeerFound(p peer.AddrInfo) {
	//fmt.Println("Found libp2p peer via mDNS:", p)
	if err := n.host.Connect(context.Background(), p); err != nil {
		fmt.Println("Error adding peer", p.ID.String(), "via mDNS:", err)
	}
	if pubkey, err := p.ID.ExtractPublicKey(); err == nil {
		raw, _ := pubkey.Raw()
		if err := n.keydb.StoreKeys(
			context.Background(),
			map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{
				gomatrixserverlib.PublicKeyLookupRequest{
					ServerName: gomatrixserverlib.ServerName(p.ID.String()),
					KeyID:      "ed25519:p2pdemo",
				}: gomatrixserverlib.PublicKeyLookupResult{
					VerifyKey: gomatrixserverlib.VerifyKey{
						Key: gomatrixserverlib.Base64String(raw),
					},
					ValidUntilTS: math.MaxUint64 >> 1,
					ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
				},
			},
		); err != nil {
			fmt.Println("Failed to store keys:", err)
		}
	}
	fmt.Println("Discovered", len(n.host.Peerstore().Peers())-1, "other libp2p peer(s):")
	for _, peer := range n.host.Peerstore().Peers() {
		if peer != n.host.ID() {
			fmt.Println("-", peer)
		}
	}
}
