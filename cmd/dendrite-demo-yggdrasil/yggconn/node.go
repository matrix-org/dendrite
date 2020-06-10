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

package yggconn

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/convert"

	"github.com/libp2p/go-yamux"
	yggdrasiladmin "github.com/yggdrasil-network/yggdrasil-go/src/admin"
	yggdrasilconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/yggdrasil"

	gologme "github.com/gologme/log"
)

type Node struct {
	core      *yggdrasil.Core
	config    *yggdrasilconfig.NodeConfig
	state     *yggdrasilconfig.NodeState
	admin     *yggdrasiladmin.AdminSocket
	multicast *yggdrasilmulticast.Multicast
	log       *gologme.Logger
	listener  *yggdrasil.Listener
	dialer    *yggdrasil.Dialer
	sessions  sync.Map // string -> yamux.Session
	incoming  chan *yamux.Stream
}

// nolint:gocyclo
func Setup(instanceName, instancePeer string) (*Node, error) {
	n := &Node{
		core:      &yggdrasil.Core{},
		config:    yggdrasilconfig.GenerateConfig(),
		admin:     &yggdrasiladmin.AdminSocket{},
		multicast: &yggdrasilmulticast.Multicast{},
		log:       gologme.New(os.Stdout, "YGG ", log.Flags()),
		incoming:  make(chan *yamux.Stream),
	}

	yggfile := fmt.Sprintf("%s-yggdrasil.conf", instanceName)
	if _, err := os.Stat(yggfile); !os.IsNotExist(err) {
		yggconf, e := ioutil.ReadFile(yggfile)
		if e != nil {
			panic(err)
		}
		if err := json.Unmarshal([]byte(yggconf), &n.config); err != nil {
			panic(err)
		}
	} else {
		n.config.AdminListen = fmt.Sprintf("unix://./%s-yggdrasil.sock", instanceName)
		n.config.MulticastInterfaces = []string{".*"}
		n.config.EncryptionPrivateKey = hex.EncodeToString(n.EncryptionPrivateKey())
		n.config.EncryptionPublicKey = hex.EncodeToString(n.EncryptionPublicKey())

		j, err := json.MarshalIndent(n.config, "", "  ")
		if err != nil {
			panic(err)
		}
		if e := ioutil.WriteFile(yggfile, j, 0600); e != nil {
			n.log.Printf("Couldn't write private key to file '%s': %s\n", yggfile, e)
		}
	}

	var err error
	n.log.EnableLevel("error")
	n.log.EnableLevel("warn")
	n.log.EnableLevel("info")
	n.state, err = n.core.Start(n.config, n.log)
	if err != nil {
		panic(err)
	}
	if instancePeer != "" {
		if err = n.core.AddPeer(instancePeer, ""); err != nil {
			panic(err)
		}
	}
	if err = n.admin.Init(n.core, n.state, n.log, nil); err != nil {
		panic(err)
	}
	if err = n.admin.Start(); err != nil {
		panic(err)
	}
	if err = n.multicast.Init(n.core, n.state, n.log, nil); err != nil {
		panic(err)
	}
	if err = n.multicast.Start(); err != nil {
		panic(err)
	}
	n.admin.SetupAdminHandlers(n.admin)
	n.multicast.SetupAdminHandlers(n.admin)
	n.listener, err = n.core.ConnListen()
	if err != nil {
		panic(err)
	}
	n.dialer, err = n.core.ConnDialer()
	if err != nil {
		panic(err)
	}

	go n.listenFromYgg()

	return n, nil
}

func (n *Node) DerivedServerName() string {
	return hex.EncodeToString(n.SigningPublicKey())
}

func (n *Node) EncryptionPublicKey() []byte {
	edkey := n.SigningPublicKey()
	return convert.Ed25519PublicKeyToCurve25519(edkey)
}

func (n *Node) EncryptionPrivateKey() []byte {
	edkey := n.SigningPrivateKey()
	return convert.Ed25519PrivateKeyToCurve25519(edkey)
}

func (n *Node) SigningPublicKey() ed25519.PublicKey {
	pubBytes, _ := hex.DecodeString(n.config.SigningPublicKey)
	return ed25519.PublicKey(pubBytes)
}

func (n *Node) SigningPrivateKey() ed25519.PrivateKey {
	privBytes, _ := hex.DecodeString(n.config.SigningPrivateKey)
	return ed25519.PrivateKey(privBytes)
}
