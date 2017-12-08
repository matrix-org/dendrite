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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/util"
)

// Default HTTPS request timeout
const requestTimeout time.Duration = time.Duration(30) * time.Second

// A Client makes request to the federation listeners of matrix
// homeservers
type Client struct {
	client http.Client
}

// UserInfo represents information about a user.
type UserInfo struct {
	Sub string `json:"sub"`
}

// NewClient makes a new Client (with default timeout)
func NewClient() *Client {
	return NewClientWithTimeout(requestTimeout)
}

// NewClientWithTimeout makes a new Client with a specified request timeout
func NewClientWithTimeout(timeout time.Duration) *Client {
	return &Client{client: http.Client{
		Transport: newFederationTripper(),
		Timeout:   timeout}}
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
					InsecureSkipVerify: true, // nolint: gas
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

	if len(dnsResult.Addrs) == 0 {
		return nil, fmt.Errorf("no address found for matrix host %v", serverName)
	}

	var resp *http.Response
	// TODO: respect the priority and weight fields from the SRV record
	for _, addr := range dnsResult.Addrs {
		u := makeHTTPSURL(r.URL, addr)
		r.URL = &u
		resp, err = f.transport.RoundTrip(r)
		if err == nil {
			return resp, nil
		}
		util.GetLogger(r.Context()).Warnf("Error sending request to %s: %v",
			u.String(), err)
	}

	// just return the most recent error
	return nil, err
}

// LookupUserInfo gets information about a user from a given matrix homeserver
// using a bearer access token.
func (fc *Client) LookupUserInfo(
	ctx context.Context, matrixServer ServerName, token string,
) (u UserInfo, err error) {
	url := url.URL{
		Scheme:   "matrix",
		Host:     string(matrixServer),
		Path:     "/_matrix/federation/v1/openid/userinfo",
		RawQuery: url.Values{"access_token": []string{token}}.Encode(),
	}

	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return
	}

	var response *http.Response
	response, err = fc.DoHTTPRequest(ctx, req)
	if response != nil {
		defer response.Body.Close() // nolint: errcheck
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

// GetServerKeys asks a matrix server for its signing keys and TLS cert
func (fc *Client) GetServerKeys(
	ctx context.Context, matrixServer ServerName,
) (ServerKeys, error) {
	url := url.URL{
		Scheme: "matrix",
		Host:   string(matrixServer),
		Path:   "/_matrix/key/v2/server",
	}

	var body ServerKeys
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return body, err
	}

	err = fc.DoRequestAndParseResponse(
		ctx, req, &body,
	)
	return body, err
}

// LookupServerKeys looks up the keys for a matrix server from a matrix server.
// The first argument is the name of the matrix server to download the keys from.
// The second argument is a map from (server name, key ID) pairs to timestamps.
// The (server name, key ID) pair identifies the key to download.
// The timestamps tell the server when the keys need to be valid until.
// Perspective servers can use that timestamp to determine whether they can
// return a cached copy of the keys or whether they will need to retrieve a fresh
// copy of the keys.
// Returns the keys returned by the server, or an error if there was a problem talking to the server.
func (fc *Client) LookupServerKeys(
	ctx context.Context, matrixServer ServerName, keyRequests map[PublicKeyRequest]Timestamp,
) ([]ServerKeys, error) {
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

	var body struct {
		ServerKeyList []ServerKeys `json:"server_keys"`
	}

	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(requestBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	err = fc.DoRequestAndParseResponse(
		ctx, req, &body,
	)
	if err != nil {
		return nil, err
	}

	return body.ServerKeyList, nil
}

// CreateMediaDownloadRequest creates a request for media on a homeserver and returns the http.Response or an error
func (fc *Client) CreateMediaDownloadRequest(
	ctx context.Context, matrixServer ServerName, mediaID string,
) (*http.Response, error) {
	requestURL := "matrix://" + string(matrixServer) + "/_matrix/media/v1/download/" + string(matrixServer) + "/" + mediaID
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, err
	}

	return fc.DoHTTPRequest(ctx, req)
}

// DoRequestAndParseResponse calls DoHTTPRequest and then decodes the response.
//
// If the HTTP response is not a 200, an attempt is made to parse the response
// body into a gomatrix.RespError. In any case, a non-200 response will result
// in a gomatrix.HTTPError.
//
func (fc *Client) DoRequestAndParseResponse(
	ctx context.Context,
	req *http.Request,
	result interface{},
) error {
	response, err := fc.DoHTTPRequest(ctx, req)
	if response != nil {
		defer response.Body.Close() // nolint: errcheck
	}
	if err != nil {
		return err
	}

	if response.StatusCode/100 != 2 { // not 2xx
		// Adapted from https://github.com/matrix-org/gomatrix/blob/master/client.go
		var contents []byte
		contents, err = ioutil.ReadAll(response.Body)
		if err != nil {
			return err
		}

		var wrap error
		var respErr gomatrix.RespError
		if _ = json.Unmarshal(contents, &respErr); respErr.ErrCode != "" {
			wrap = respErr
		}

		// If we failed to decode as RespError, don't just drop the HTTP body, include it in the
		// HTTP error instead (e.g proxy errors which return HTML).
		msg := "Failed to " + req.Method + " JSON to " + req.RequestURI
		if wrap == nil {
			msg = msg + ": " + string(contents)
		}

		return gomatrix.HTTPError{
			Code:         response.StatusCode,
			Message:      msg,
			WrappedError: wrap,
		}
	}

	if err = json.NewDecoder(response.Body).Decode(result); err != nil {
		return err
	}

	return nil
}

// DoHTTPRequest creates an outgoing request ID and adds it to the context
// before sending off the request and awaiting a response.
//
// If the returned error is nil, the Response will contain a non-nil
// Body which the caller is expected to close.
//
func (fc *Client) DoHTTPRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	reqID := util.RandomString(12)
	logger := util.GetLogger(ctx).WithField("server", req.URL.Host).WithField("out.req.ID", reqID)
	newCtx := util.ContextWithLogger(ctx, logger)

	logger.Infof("Outgoing request %s %s", req.Method, req.URL)
	resp, err := fc.client.Do(req.WithContext(newCtx))
	if err != nil {
		logger.Infof("Outgoing request %s %s failed with %v", req.Method, req.URL, err)
		return nil, err
	}

	// we haven't yet read the body, so this is slightly premature, but it's the easiest place.
	logger.Infof("Response %d from %s %s", resp.StatusCode, req.Method, req.URL)

	return resp, nil
}
