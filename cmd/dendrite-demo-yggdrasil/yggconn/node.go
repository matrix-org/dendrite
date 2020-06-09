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

	"github.com/libp2p/go-yamux"
	yggdrasiladmin "github.com/yggdrasil-network/yggdrasil-go/src/admin"
	yggdrasilconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	"github.com/yggdrasil-network/yggdrasil-go/src/yggdrasil"

	gologme "github.com/gologme/log"
)

type Node struct {
	core      yggdrasil.Core
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

func Setup(instanceName string) (*Node, error) {
	n := Node{
		config:    yggdrasilconfig.GenerateConfig(),
		admin:     &yggdrasiladmin.AdminSocket{},
		multicast: &yggdrasilmulticast.Multicast{},
		log:       gologme.New(os.Stdout, "YGG ", log.Flags()),
		incoming:  make(chan *yamux.Stream),
	}
	n.config.AdminListen = fmt.Sprintf("unix://./%s-yggdrasil.sock", instanceName)

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
		j, err := json.Marshal(n.config)
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
	n.core.UpdateConfig(n.config)
	if err = n.admin.Init(&n.core, n.state, n.log, nil); err != nil {
		panic(err)
	}
	if err = n.admin.Start(); err != nil {
		panic(err)
	}
	if err = n.multicast.Init(&n.core, n.state, n.log, nil); err != nil {
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

	return &n, nil
}

func (n *Node) EncryptionPublicKey() string {
	return n.core.EncryptionPublicKey()
}

func (n *Node) SigningPrivateKey() ed25519.PrivateKey {
	privBytes, _ := hex.DecodeString(n.config.SigningPrivateKey)
	return ed25519.PrivateKey(privBytes)
}
