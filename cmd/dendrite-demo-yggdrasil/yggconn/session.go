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
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yggdrasil-network/yggdrasil-go/src/crypto"
	"github.com/yggdrasil-network/yggdrasil-go/src/yggdrasil"
)

type session struct {
	node    *Node
	session quic.Session
	address string
	context context.Context
	cancel  context.CancelFunc
}

func (n *Node) newSession(sess quic.Session, address string) *session {
	ctx, cancel := context.WithCancel(context.TODO())
	return &session{
		node:    n,
		session: sess,
		address: address,
		context: ctx,
		cancel:  cancel,
	}
}

func (s *session) kill() {
	s.cancel()
}

func (n *Node) listenFromYgg() {
	var err error
	n.listener, err = quic.Listen(
		n.core,       // yggdrasil.PacketConn
		n.tlsConfig,  // TLS config
		n.quicConfig, // QUIC config
	)
	if err != nil {
		panic(err)
	}

	for {
		n.log.Infoln("Waiting to accept QUIC sessions")
		session, err := n.listener.Accept(context.TODO())
		if err != nil {
			n.log.Println("n.listener.Accept:", err)
			return
		}
		if len(session.ConnectionState().PeerCertificates) != 1 {
			_ = session.CloseWithError(0, "expected a peer certificate")
			continue
		}
		address := session.ConnectionState().PeerCertificates[0].DNSNames[0]
		n.log.Infoln("Accepted connection from", address)
		go n.newSession(session, address).listenFromQUIC()
		go n.sessionFunc(address)
	}
}

func (s *session) listenFromQUIC() {
	if existing, ok := s.node.sessions.Load(s.address); ok {
		if existingSession, ok := existing.(*session); ok {
			fmt.Println("Killing existing session to replace", s.address)
			existingSession.kill()
		}
	}
	s.node.sessionCount.Inc()
	s.node.sessions.Store(s.address, s)
	defer s.node.sessions.Delete(s.address)
	defer s.node.sessionCount.Dec()
	for {
		st, err := s.session.AcceptStream(s.context)
		if err != nil {
			s.node.log.Println("session.AcceptStream:", err)
			return
		}
		s.node.incoming <- QUICStream{st, s.session}
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
// nolint:gocyclo
func (n *Node) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	s, ok1 := n.sessions.Load(address)
	session, ok2 := s.(*session)
	if !ok1 || !ok2 {
		// First of all, check if we think we know the coords of this
		// node. If we do then we'll try to dial to it directly. This
		// will either succeed or fail.
		if v, ok := n.coords.Load(address); ok {
			coords, ok := v.(yggdrasil.Coords)
			if !ok {
				n.coords.Delete(address)
				return nil, errors.New("should have found yggdrasil.Coords but didn't")
			}
			n.log.Infof("Coords %s for %q cached, trying to dial", coords.String(), address)
			var err error
			// We think we know the coords. Try to dial the node.
			if session, err = n.tryDial(address, coords); err != nil {
				// We thought we knew the coords but it didn't result
				// in a successful dial. Nuke them from the cache.
				n.coords.Delete(address)
				n.log.Infof("Cached coords %s for %q failed", coords.String(), address)
			}
		}

		// We either don't know the coords for the node, or we failed
		// to dial it before, in which case try to resolve the coords.
		if _, ok := n.coords.Load(address); !ok {
			var coords yggdrasil.Coords
			var err error

			// First look and see if the node is something that we already
			// know about from our direct switch peers.
			for _, peer := range n.core.GetSwitchPeers() {
				if peer.PublicKey.String() == address {
					coords = peer.Coords
					n.log.Infof("%q is a direct peer, coords are %s", address, coords.String())
					n.coords.Store(address, coords)
					break
				}
			}

			// If it isn' a node that we know directly then try to search
			// the network.
			if coords == nil {
				n.log.Infof("Searching for coords for %q", address)
				dest, derr := hex.DecodeString(address)
				if derr != nil {
					return nil, derr
				}
				if len(dest) != crypto.BoxPubKeyLen {
					return nil, errors.New("invalid key length supplied")
				}
				var pubKey crypto.BoxPubKey
				copy(pubKey[:], dest)
				nodeID := crypto.GetNodeID(&pubKey)
				nodeMask := &crypto.NodeID{}
				for i := range nodeMask {
					nodeMask[i] = 0xFF
				}

				fmt.Println("Resolving coords")
				coords, err = n.core.Resolve(nodeID, nodeMask)
				if err != nil {
					return nil, fmt.Errorf("n.core.Resolve: %w", err)
				}
				fmt.Println("Found coords:", coords)
				n.coords.Store(address, coords)
			}

			// We now know the coords in theory. Let's try dialling the
			// node again.
			if session, err = n.tryDial(address, coords); err != nil {
				return nil, fmt.Errorf("n.tryDial: %w", err)
			}
		}
	}

	if session == nil {
		return nil, fmt.Errorf("should have found session but didn't")
	}

	st, err := session.session.OpenStream()
	if err != nil {
		n.log.Println("session.OpenStream:", err)
		_ = session.session.CloseWithError(0, "expected to be able to open session")
		return nil, err
	}
	return QUICStream{st, session.session}, nil
}

func (n *Node) tryDial(address string, coords yggdrasil.Coords) (*session, error) {
	quicSession, err := quic.Dial(
		n.core,       // yggdrasil.PacketConn
		coords,       // dial address
		address,      // dial SNI
		n.tlsConfig,  // TLS config
		n.quicConfig, // QUIC config
	)
	if err != nil {
		return nil, err
	}
	if len(quicSession.ConnectionState().PeerCertificates) != 1 {
		_ = quicSession.CloseWithError(0, "expected a peer certificate")
		return nil, errors.New("didn't receive a peer certificate")
	}
	if len(quicSession.ConnectionState().PeerCertificates[0].DNSNames) != 1 {
		_ = quicSession.CloseWithError(0, "expected a DNS name")
		return nil, errors.New("didn't receive a DNS name")
	}
	if gotAddress := quicSession.ConnectionState().PeerCertificates[0].DNSNames[0]; address != gotAddress {
		_ = quicSession.CloseWithError(0, "you aren't the host I was hoping for")
		return nil, fmt.Errorf("expected %q but dialled %q", address, gotAddress)
	}
	session := n.newSession(quicSession, address)
	go session.listenFromQUIC()
	go n.sessionFunc(address)
	return session, nil
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
		ClientAuth:         tls.RequireAnyClientCert,
		GetClientCertificate: func(info *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &tlsCert, nil
		},
	}
}
