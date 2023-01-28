// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package gobind

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/api"
	relayServerAPI "github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gotest.tools/v3/poll"
)

var TestBuf = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

type TestNetConn struct {
	net.Conn
	shouldFail bool
}

func (t *TestNetConn) Read(b []byte) (int, error) {
	if t.shouldFail {
		return 0, fmt.Errorf("Failed")
	} else {
		n := copy(b, TestBuf)
		return n, nil
	}
}

func (t *TestNetConn) Write(b []byte) (int, error) {
	if t.shouldFail {
		return 0, fmt.Errorf("Failed")
	} else {
		return len(b), nil
	}
}

func (t *TestNetConn) Close() error {
	if t.shouldFail {
		return fmt.Errorf("Failed")
	} else {
		return nil
	}
}

func TestConduitStoresPort(t *testing.T) {
	conduit := Conduit{port: 7}
	assert.Equal(t, 7, conduit.Port())
}

func TestConduitRead(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	b := make([]byte, len(TestBuf))
	bytes, err := conduit.Read(b)
	assert.NoError(t, err)
	assert.Equal(t, len(TestBuf), bytes)
	assert.Equal(t, TestBuf, b)
}

func TestConduitReadCopy(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	result, err := conduit.ReadCopy()
	assert.NoError(t, err)
	assert.Equal(t, TestBuf, result)
}

func TestConduitWrite(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	bytes, err := conduit.Write(TestBuf)
	assert.NoError(t, err)
	assert.Equal(t, len(TestBuf), bytes)
}

func TestConduitClose(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	assert.True(t, conduit.closed.Load())
}

func TestConduitReadClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	b := make([]byte, len(TestBuf))
	_, err = conduit.Read(b)
	assert.Error(t, err)
}

func TestConduitReadCopyClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	_, err = conduit.ReadCopy()
	assert.Error(t, err)
}

func TestConduitWriteClosed(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{}}
	err := conduit.Close()
	assert.NoError(t, err)
	_, err = conduit.Write(TestBuf)
	assert.Error(t, err)
}

func TestConduitReadCopyFails(t *testing.T) {
	conduit := Conduit{conn: &TestNetConn{shouldFail: true}}
	_, err := conduit.ReadCopy()
	assert.Error(t, err)
}

var testRelayServers = []gomatrixserverlib.ServerName{"relay1", "relay2"}

type FakeFedAPI struct {
	api.FederationInternalAPI
}

func (f *FakeFedAPI) P2PQueryRelayServers(ctx context.Context, req *api.P2PQueryRelayServersRequest, res *api.P2PQueryRelayServersResponse) error {
	res.RelayServers = testRelayServers
	return nil
}

type FakeRelayAPI struct {
	relayServerAPI.RelayInternalAPI
}

func (r *FakeRelayAPI) PerformRelayServerSync(ctx context.Context, userID gomatrixserverlib.UserID, relayServer gomatrixserverlib.ServerName) error {
	return nil
}

func TestRelayRetrieverInitialization(t *testing.T) {
	retriever := RelayServerRetriever{
		Context:             context.Background(),
		ServerName:          "server",
		relayServersQueried: make(map[gomatrixserverlib.ServerName]bool),
		FederationAPI:       &FakeFedAPI{},
		RelayAPI:            &FakeRelayAPI{},
	}

	retriever.InitializeRelayServers(logrus.WithField("test", "relay"))
	relayServers := retriever.GetQueriedServerStatus()
	assert.Equal(t, 2, len(relayServers))
}

func TestRelayRetrieverSync(t *testing.T) {
	retriever := RelayServerRetriever{
		Context:             context.Background(),
		ServerName:          "server",
		relayServersQueried: make(map[gomatrixserverlib.ServerName]bool),
		FederationAPI:       &FakeFedAPI{},
		RelayAPI:            &FakeRelayAPI{},
	}

	retriever.InitializeRelayServers(logrus.WithField("test", "relay"))
	relayServers := retriever.GetQueriedServerStatus()
	assert.Equal(t, 2, len(relayServers))

	stopRelayServerSync := make(chan bool)
	go retriever.SyncRelayServers(stopRelayServerSync)

	check := func(log poll.LogT) poll.Result {
		relayServers := retriever.GetQueriedServerStatus()
		for _, queried := range relayServers {
			if !queried {
				return poll.Continue("waiting for all servers to be queried")
			}
		}

		stopRelayServerSync <- true
		return poll.Success()
	}
	poll.WaitOn(t, check, poll.WithTimeout(5*time.Second), poll.WithDelay(100*time.Millisecond))
}

func TestMonolithStarts(t *testing.T) {
	monolith := DendriteMonolith{}
	monolith.Start()
	monolith.PublicKey()
	monolith.Stop()
}

func TestMonolithSetRelayServers(t *testing.T) {
	// "valid,invalid,,valid"
	// ensure both valids and only them exist
	// test with both valid & invalid nodeID input
	// test with both self & remote nodeID
	testCases := []struct {
		name           string
		nodeID         string
		relays         string
		expectedRelays string
	}{
		{
			name:           "assorted valid, invalid, empty & self keys",
			nodeID:         "@valid:abcdef123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
			relays:         "@valid:123456123456abcdef123456abcdef123456abcdef123456abcdef123456abcd,@invalid:notakey,,",
			expectedRelays: "123456123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
		},
	}

	for _, tc := range testCases {
		monolith := DendriteMonolith{}
		monolith.Start()
		inputRelays := tc.relays + "," + monolith.PublicKey()
		expectedRelays := tc.expectedRelays + "," + monolith.PublicKey()
		monolith.SetRelayServers(tc.nodeID, inputRelays)

		relays := monolith.GetRelayServers(tc.nodeID)
		monolith.Stop()

		if relays != expectedRelays {
			t.Fatalf("%s: expected %s got %s", tc.name, expectedRelays, relays)
		}
	}
}

func TestParseServerKey(t *testing.T) {
	testCases := []struct {
		name        string
		serverKey   string
		expectedErr bool
		expectedKey gomatrixserverlib.ServerName
	}{
		{
			name:        "valid userid as key",
			serverKey:   "@valid:abcdef123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
			expectedErr: false,
			expectedKey: "abcdef123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
		},
		{
			name:        "valid key",
			serverKey:   "abcdef123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
			expectedErr: false,
			expectedKey: "abcdef123456abcdef123456abcdef123456abcdef123456abcdef123456abcd",
		},
		{
			name:        "invalid userid key",
			serverKey:   "@invalid:notakey",
			expectedErr: true,
			expectedKey: "",
		},
		{
			name:        "invalid key",
			serverKey:   "@invalid:notakey",
			expectedErr: true,
			expectedKey: "",
		},
	}

	for _, tc := range testCases {
		key, err := getServerKeyFromString(tc.serverKey)
		if tc.expectedErr && err == nil {
			t.Fatalf("%s: expected an error", tc.name)
		} else if !tc.expectedErr && err != nil {
			t.Fatalf("%s: didn't expect an error: %s", tc.name, err.Error())
		}
		if tc.expectedKey != key {
			t.Fatalf("%s: keys not equal. expected: %s got: %s", tc.name, tc.expectedKey, key)
		}
	}
}
