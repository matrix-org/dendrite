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
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/convert"
	"github.com/matrix-org/gomatrixserverlib"
	"go.uber.org/atomic"

	yggdrasilconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/yggdrasil"

	gologme "github.com/gologme/log"
)

type Node struct {
	core         *yggdrasil.Core
	config       *yggdrasilconfig.NodeConfig
	state        *yggdrasilconfig.NodeState
	multicast    *yggdrasilmulticast.Multicast
	log          *gologme.Logger
	listener     quic.Listener
	tlsConfig    *tls.Config
	quicConfig   *quic.Config
	sessions     sync.Map // string -> *session
	sessionCount atomic.Uint32
	sessionFunc  func(address string)
	coords       sync.Map // string -> yggdrasil.Coords
	incoming     chan QUICStream
	NewSession   func(remote gomatrixserverlib.ServerName)
}

func (n *Node) Dialer(_, address string) (net.Conn, error) {
	tokens := strings.Split(address, ":")
	raw, err := hex.DecodeString(tokens[0])
	if err != nil {
		return nil, fmt.Errorf("hex.DecodeString: %w", err)
	}
	converted := convert.Ed25519PublicKeyToCurve25519(ed25519.PublicKey(raw))
	convhex := hex.EncodeToString(converted)
	return n.Dial("curve25519", convhex)
}

func (n *Node) DialerContext(ctx context.Context, network, address string) (net.Conn, error) {
	return n.Dialer(network, address)
}

// nolint:gocyclo
func Setup(instanceName, storageDirectory string) (*Node, error) {
	n := &Node{
		core:      &yggdrasil.Core{},
		config:    yggdrasilconfig.GenerateConfig(),
		multicast: &yggdrasilmulticast.Multicast{},
		log:       gologme.New(os.Stdout, "YGG ", log.Flags()),
		incoming:  make(chan QUICStream),
	}

	yggfile := fmt.Sprintf("%s/%s-yggdrasil.conf", storageDirectory, instanceName)
	if _, err := os.Stat(yggfile); !os.IsNotExist(err) {
		yggconf, e := ioutil.ReadFile(yggfile)
		if e != nil {
			panic(err)
		}
		if err := json.Unmarshal([]byte(yggconf), &n.config); err != nil {
			panic(err)
		}
	}

	n.core.SetCoordChangeCallback(func(old, new yggdrasil.Coords) {
		fmt.Println("COORDINATE CHANGE!")
		fmt.Println("Old:", old)
		fmt.Println("New:", new)
		n.sessions.Range(func(k, v interface{}) bool {
			if s, ok := v.(*session); ok {
				fmt.Println("Killing session", k)
				s.kill()
			}
			return true
		})
	})

	n.config.Peers = []string{}
	n.config.AdminListen = "none"
	n.config.MulticastInterfaces = []string{}
	n.config.EncryptionPrivateKey = hex.EncodeToString(n.EncryptionPrivateKey())
	n.config.EncryptionPublicKey = hex.EncodeToString(n.EncryptionPublicKey())

	j, err := json.MarshalIndent(n.config, "", "  ")
	if err != nil {
		panic(err)
	}
	if e := ioutil.WriteFile(yggfile, j, 0600); e != nil {
		n.log.Printf("Couldn't write private key to file '%s': %s\n", yggfile, e)
	}

	n.log.EnableLevel("error")
	n.log.EnableLevel("warn")
	n.log.EnableLevel("info")
	n.state, err = n.core.Start(n.config, n.log)
	if err != nil {
		panic(err)
	}
	if err = n.multicast.Init(n.core, n.state, n.log, nil); err != nil {
		panic(err)
	}
	if err = n.multicast.Start(); err != nil {
		panic(err)
	}

	n.tlsConfig = n.generateTLSConfig()
	n.quicConfig = &quic.Config{
		MaxIncomingStreams:    0,
		MaxIncomingUniStreams: 0,
		KeepAlive:             true,
		MaxIdleTimeout:        time.Minute * 30,
		HandshakeTimeout:      time.Second * 15,
	}
	copy(n.quicConfig.StatelessResetKey, n.EncryptionPublicKey())

	n.log.Println("Public curve25519:", n.core.EncryptionPublicKey())
	n.log.Println("Public ed25519:", n.core.SigningPublicKey())

	go func() {
		time.Sleep(time.Second)
		n.listenFromYgg()
	}()

	return n, nil
}

func (n *Node) Stop() {
	if err := n.multicast.Stop(); err != nil {
		n.log.Println("Error stopping multicast:", err)
	}
	n.core.Stop()
}

func (n *Node) DerivedServerName() string {
	return hex.EncodeToString(n.SigningPublicKey())
}

func (n *Node) DerivedSessionName() string {
	return hex.EncodeToString(n.EncryptionPublicKey())
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

func (n *Node) SetSessionFunc(f func(address string)) {
	n.sessionFunc = f
}

func (n *Node) PeerCount() int {
	return len(n.core.GetPeers()) - 1
}

func (n *Node) SessionCount() int {
	return int(n.sessionCount.Load())
}

func (n *Node) KnownNodes() []gomatrixserverlib.ServerName {
	nodemap := map[string]struct{}{
		//"b5ae50589e50991dd9dd7d59c5c5f7a4521e8da5b603b7f57076272abc58b374": {},
	}
	for _, peer := range n.core.GetSwitchPeers() {
		nodemap[hex.EncodeToString(peer.SigPublicKey[:])] = struct{}{}
	}
	n.sessions.Range(func(_, v interface{}) bool {
		session, ok := v.(quic.Session)
		if !ok {
			return true
		}
		if len(session.ConnectionState().PeerCertificates) != 1 {
			return true
		}
		subjectName := session.ConnectionState().PeerCertificates[0].Subject.CommonName
		nodemap[subjectName] = struct{}{}
		return true
	})
	var nodes []gomatrixserverlib.ServerName
	for node := range nodemap {
		nodes = append(nodes, gomatrixserverlib.ServerName(node))
	}
	return nodes
}

func (n *Node) SetMulticastEnabled(enabled bool) {
	if enabled {
		n.config.MulticastInterfaces = []string{".*"}
	} else {
		n.config.MulticastInterfaces = []string{}
	}
	n.multicast.UpdateConfig(n.config)
	if !enabled {
		n.DisconnectMulticastPeers()
	}
}

func (n *Node) DisconnectMulticastPeers() {
	for _, sp := range n.core.GetSwitchPeers() {
		if !strings.HasPrefix(sp.Endpoint, "fe80") {
			continue
		}
		if err := n.core.DisconnectPeer(sp.Port); err != nil {
			n.log.Printf("Failed to disconnect port %d: %s", sp.Port, err)
		}
	}
}

func (n *Node) DisconnectNonMulticastPeers() {
	for _, sp := range n.core.GetSwitchPeers() {
		if strings.HasPrefix(sp.Endpoint, "fe80") {
			continue
		}
		if err := n.core.DisconnectPeer(sp.Port); err != nil {
			n.log.Printf("Failed to disconnect port %d: %s", sp.Port, err)
		}
	}
}

func (n *Node) SetStaticPeer(uri string) error {
	n.config.Peers = []string{}
	n.core.UpdateConfig(n.config)
	n.DisconnectNonMulticastPeers()
	if uri != "" {
		n.log.Infoln("Adding static peer", uri)
		if err := n.core.AddPeer(uri, ""); err != nil {
			n.log.Warnln("Adding static peer failed:", err)
			return err
		}
		if err := n.core.CallPeer(uri, ""); err != nil {
			n.log.Warnln("Calling static peer failed:", err)
			return err
		}
	}
	return nil
}
