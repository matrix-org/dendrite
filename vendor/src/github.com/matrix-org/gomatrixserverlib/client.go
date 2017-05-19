/* Copyright 2016-2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// A Client makes request to the federation listeners of matrix
// homeservers
type Client struct {
	client http.Client
}

// UserInfo represents information about a user.
type UserInfo struct {
	Sub string `json:"sub"`
}

// NewClient makes a new Client
func NewClient() *Client {
	return &Client{client: http.Client{Transport: newFederationTripper()}}
}

type federationTripper struct {
	transport http.RoundTripper
}

func newFederationTripper() *federationTripper {
	// TODO: Verify ceritificates
	return &federationTripper{
		transport: &http.Transport{
			// Set our own DialTLS function to avoid the default net/http SNI.
			// By default net/http and crypto/tls set the SNI to the target host.
			// By avoiding the default implementation we can keep the ServerName
			// as the empty string so that crypto/tls doesn't add SNI.
			DialTLS: func(network, addr string) (net.Conn, error) {
				rawconn, err := net.Dial(network, addr)
				if err != nil {
					return nil, err
				}
				// Wrap a raw connection ourselves since tls.Dial defaults the SNI
				conn := tls.Client(rawconn, &tls.Config{
					ServerName: "",
					// TODO: We should be checking that the TLS certificate we see here matches
					//       one of the allowed SHA-256 fingerprints for the server.
					InsecureSkipVerify: true,
				})
				if err := conn.Handshake(); err != nil {
					return nil, err
				}
				return conn, nil
			},
		},
	}
}

func makeHTTPSURL(u *url.URL, addr string) (httpsURL url.URL) {
	httpsURL = *u
	httpsURL.Scheme = "https"
	httpsURL.Host = addr
	return
}

func (f *federationTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	serverName := ServerName(r.URL.Host)
	dnsResult, err := LookupServer(serverName)
	if err != nil {
		return nil, err
	}
	var resp *http.Response
	for _, addr := range dnsResult.Addrs {
		u := makeHTTPSURL(r.URL, addr)
		r.URL = &u
		resp, err = f.transport.RoundTrip(r)
		if err == nil {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("no address found for matrix host %v", serverName)
}

// LookupUserInfo gets information about a user from a given matrix homeserver
// using a bearer access token.
func (fc *Client) LookupUserInfo(matrixServer ServerName, token string) (u UserInfo, err error) {
	url := url.URL{
		Scheme:   "matrix",
		Host:     string(matrixServer),
		Path:     "/_matrix/federation/v1/openid/userinfo",
		RawQuery: url.Values{"access_token": []string{token}}.Encode(),
	}

	var response *http.Response
	response, err = fc.client.Get(url.String())
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errorOutput []byte
		errorOutput, err = ioutil.ReadAll(response.Body)
		if err != nil {
			return
		}
		err = fmt.Errorf("HTTP %d : %s", response.StatusCode, errorOutput)
		return
	}

	err = json.NewDecoder(response.Body).Decode(&u)
	if err != nil {
		return
	}

	userParts := strings.SplitN(u.Sub, ":", 2)
	if len(userParts) != 2 || userParts[1] != string(matrixServer) {
		err = fmt.Errorf("userID doesn't match server name '%v' != '%v'", u.Sub, matrixServer)
		return
	}

	return
}

// LookupServerKeys lookups up the keys for a matrix server from a matrix server.
// The first argument is the name of the matrix server to download the keys from.
// The second argument is a map from (server name, key ID) pairs to timestamps.
// The (server name, key ID) pair identifies the key to download.
// The timestamps tell the server when the keys need to be valid until.
// Perspective servers can use that timestamp to determine whether they can
// return a cached copy of the keys or whether they will need to retrieve a fresh
// copy of the keys.
// Returns the keys or an error if there was a problem talking to the server.
func (fc *Client) LookupServerKeys(
	matrixServer ServerName, keyRequests map[PublicKeyRequest]Timestamp,
) (map[PublicKeyRequest]ServerKeys, error) {
	url := url.URL{
		Scheme: "matrix",
		Host:   string(matrixServer),
		Path:   "/_matrix/key/v2/query",
	}

	// The request format is:
	// { "server_keys": { "<server_name>": { "<key_id>": { "minimum_valid_until_ts": <ts> }}}
	type keyreq struct {
		MinimumValidUntilTS Timestamp `json:"minimum_valid_until_ts"`
	}
	request := struct {
		ServerKeyMap map[ServerName]map[KeyID]keyreq `json:"server_keys"`
	}{map[ServerName]map[KeyID]keyreq{}}
	for k, ts := range keyRequests {
		server := request.ServerKeyMap[k.ServerName]
		if server == nil {
			server = map[KeyID]keyreq{}
			request.ServerKeyMap[k.ServerName] = server
		}
		server[k.KeyID] = keyreq{ts}
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	response, err := fc.client.Post(url.String(), "application/json", bytes.NewBuffer(requestBytes))
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	if response.StatusCode != 200 {
		var errorOutput []byte
		if errorOutput, err = ioutil.ReadAll(response.Body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("HTTP %d : %s", response.StatusCode, errorOutput)
	}

	var body struct {
		ServerKeyList []ServerKeys `json:"server_keys"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, err
	}

	result := map[PublicKeyRequest]ServerKeys{}
	for _, keys := range body.ServerKeyList {
		keys.FromServer = matrixServer
		// TODO: What happens if the same key ID appears in multiple responses?
		// We should probably take the response with the highest valid_until_ts.
		for keyID := range keys.VerifyKeys {
			result[PublicKeyRequest{keys.ServerName, keyID}] = keys
		}
		for keyID := range keys.OldVerifyKeys {
			result[PublicKeyRequest{keys.ServerName, keyID}] = keys
		}
	}
	return result, nil
}
