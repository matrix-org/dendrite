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
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/yggdrasil-network/yggdrasil-go/src/crypto"
)

func (n *Node) listenFromYgg() {
	var err error
	n.listener, err = quic.Listen(
		n.packetConn, // yggdrasil.PacketConn
		n.tlsConfig,  // TLS config
		n.quicConfig, // QUIC config
	)
	if err != nil {
		panic(err)
	}

	for {
		session, err := n.listener.Accept(context.TODO())
		if err != nil {
			n.log.Println("n.listener.Accept:", err)
			return
		}
		go n.listenFromQUIC(session)
	}
}

func (n *Node) listenFromQUIC(session quic.Session) {
	n.sessions.Store(session.RemoteAddr().String(), session)
	defer n.sessions.Delete(session.RemoteAddr())
	if n.NewSession != nil {
		if len(session.ConnectionState().PeerCertificates) == 1 {
			subjectName := session.ConnectionState().PeerCertificates[0].Subject.CommonName
			go n.NewSession(gomatrixserverlib.ServerName(subjectName))
		}
	}
	for {
		st, err := session.AcceptStream(context.TODO())
		if err != nil {
			n.log.Println("session.AcceptStream:", err)
			return
		}
		n.incoming <- QUICStream{st, session}
	}
}

// Implements net.Listener
func (n *Node) Accept() (net.Conn, error) {
	return <-n.incoming, nil
}

// Implements net.Listener
func (n *Node) Close() error {
	return n.listener.Close()
}

// Implements net.Listener
func (n *Node) Addr() net.Addr {
	return n.listener.Addr()
}

// Implements http.Transport.Dial
func (n *Node) Dial(network, address string) (net.Conn, error) {
	return n.DialContext(context.TODO(), network, address)
}

// Implements http.Transport.DialContext
func (n *Node) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	s, ok1 := n.sessions.Load(address)
	session, ok2 := s.(quic.Session)
	if !ok1 || !ok2 || (ok1 && ok2 && session.ConnectionState().HandshakeComplete) {
		dest, err := hex.DecodeString(address)
		if err != nil {
			return nil, err
		}
		if len(dest) != crypto.BoxPubKeyLen {
			return nil, errors.New("invalid key length supplied")
		}
		var pubKey crypto.BoxPubKey
		copy(pubKey[:], dest)

		session, err = quic.Dial(
			n.packetConn, // yggdrasil.PacketConn
			&pubKey,      // dial address
			address,      // dial SNI
			n.tlsConfig,  // TLS config
			n.quicConfig, // QUIC config
		)
		if err != nil {
			n.log.Println("n.dialer.DialContext:", err)
			return nil, err
		}
		go n.listenFromQUIC(session)
	}
	st, err := session.OpenStream()
	if err != nil {
		n.log.Println("session.OpenStream:", err)
		return nil, err
	}
	return QUICStream{st, session}, nil
}

func (n *Node) generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		Subject: pkix.Name{
			CommonName: n.DerivedServerName(),
		},
		SerialNumber: big.NewInt(1),
		NotAfter:     time.Now().Add(time.Hour * 24 * 365),
		DNSNames:     []string{n.DerivedSessionName()},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"quic-matrix-ygg"},
		InsecureSkipVerify: true,
	}
}
