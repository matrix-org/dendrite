package gomatrixserverlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/matrix-org/util"
	"golang.org/x/crypto/ed25519"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

// A FederationRequest is a request to send to a remote server or a request
// received from a remote server.
// Federation requests are signed by building a JSON object and signing it
type FederationRequest struct {
	// fields implement the JSON format needed for signing
	// specified in https://matrix.org/docs/spec/server_server/unstable.html#request-authentication
	fields struct {
		Content     rawJSON                         `json:"content,omitempty"`
		Destination ServerName                      `json:"destination"`
		Method      string                          `json:"method"`
		Origin      ServerName                      `json:"origin"`
		RequestURI  string                          `json:"uri"`
		Signatures  map[ServerName]map[KeyID]string `json:"signatures,omitempty"`
	}
}

// NewFederationRequest creates a matrix request. Takes an HTTP method, a
// destination homeserver and a request path which can have a query string.
// The destination is the name of a matrix homeserver.
// The request path must begin with a slash.
// Eg. NewFederationRequest("GET", "matrix.org", "/_matrix/federation/v1/send/123")
func NewFederationRequest(method string, destination ServerName, requestURI string) FederationRequest {
	var r FederationRequest
	r.fields.Destination = destination
	r.fields.Method = strings.ToUpper(method)
	r.fields.RequestURI = requestURI
	return r
}

// SetContent sets the JSON content for the request.
// Returns an error if there already is JSON content present on the request.
func (r *FederationRequest) SetContent(content interface{}) error {
	if r.fields.Content != nil {
		return fmt.Errorf("gomatrixserverlib: content already set on the request")
	}
	if r.fields.Signatures != nil {
		return fmt.Errorf("gomatrixserverlib: the request is signed and cannot be modified")
	}
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	r.fields.Content = rawJSON(data)
	return nil
}

// Method returns the JSON method for the request.
func (r *FederationRequest) Method() string {
	return r.fields.Method
}

// Content returns the JSON content for the request.
func (r *FederationRequest) Content() []byte {
	return []byte(r.fields.Content)
}

// Origin returns the server that the request originated on.
func (r *FederationRequest) Origin() ServerName {
	return r.fields.Origin
}

// RequestURI returns the path and query sections of the HTTP request URL.
func (r *FederationRequest) RequestURI() string {
	return r.fields.RequestURI
}

// Sign the matrix request with an ed25519 key.
// Uses the algorithm specified https://matrix.org/docs/spec/server_server/unstable.html#request-authentication
// Updates the request with the signature in place.
// Returns an error if there was a problem signing the request.
func (r *FederationRequest) Sign(serverName ServerName, keyID KeyID, privateKey ed25519.PrivateKey) error {
	if r.fields.Origin != "" && r.fields.Origin != serverName {
		return fmt.Errorf("gomatrixserverlib: the request is already signed by a different server")
	}
	r.fields.Origin = serverName
	// The request fields are already in the form required by the specification
	// So we can just serialise the request fields using the default marshaller
	data, err := json.Marshal(r.fields)
	if err != nil {
		return err
	}
	signedData, err := SignJSON(string(serverName), keyID, privateKey, data)
	if err != nil {
		return err
	}
	// Now we can deserialise the signed request back into the request structure
	// to set the Signatures field, (This will clobber the other fields but they
	// will all round-trip through an encode/decode.)
	return json.Unmarshal(signedData, &r.fields)
}

// HTTPRequest constructs an net/http.Request for this matrix request.
// The request can be passed to net/http.Client.Do().
func (r *FederationRequest) HTTPRequest() (*http.Request, error) {
	urlStr := fmt.Sprintf("matrix://%s%s", r.fields.Destination, r.fields.RequestURI)

	var content io.Reader
	if r.fields.Content != nil {
		content = bytes.NewReader([]byte(r.fields.Content))
	}

	httpReq, err := http.NewRequest(r.fields.Method, urlStr, content)
	if err != nil {
		return nil, err
	}

	// Sanity check that the request fields will round-trip properly.
	if httpReq.URL.RequestURI() != r.fields.RequestURI {
		return nil, fmt.Errorf(
			"gomatrixserverlib: Request URI didn't encode properly. Wanted %q. Got %q",
			r.fields.RequestURI, httpReq.URL.RequestURI(),
		)
	}

	if r.fields.Content != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	for keyID, sig := range r.fields.Signatures[r.fields.Origin] {
		// Check that we can safely include the origin and key ID in the header.
		// We don't need to check the signature since we already know that it is
		// base64.
		if !isSafeInHTTPQuotedString(string(r.fields.Origin)) {
			return nil, fmt.Errorf("gomatrixserverlib: Request Origin isn't safe to include in an HTTP header")
		}
		if !isSafeInHTTPQuotedString(string(keyID)) {
			return nil, fmt.Errorf("gomatrixserverlib: Request key ID isn't safe to include in an HTTP header")
		}
		httpReq.Header.Add("Authorization", fmt.Sprintf(
			"X-Matrix origin=\"%s\",key=\"%s\",sig=\"%s\"", r.fields.Origin, keyID, sig,
		))
	}

	return httpReq, nil
}

// isSafeInHTTPQuotedString checks whether the string is safe to include
// in an HTTP quoted-string without escaping.
// According to https://tools.ietf.org/html/rfc7230#section-3.2.6 the safe
// charcters are:
//
//    qdtext         = HTAB / SP / %x21 / %x23-5B / %x5D-7E / %x80-FF
//
func isSafeInHTTPQuotedString(text string) bool {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\t':
			continue
		case c == ' ':
			continue
		case c == 0x21:
			continue
		case 0x23 <= c && c <= 0x5B:
			continue
		case 0x5D <= c && c <= 0x7E:
			continue
		case 0x80 <= c && c <= 0xFF:
			continue
		default:
			return false
		}
	}
	return true
}

// VerifyHTTPRequest extracts and verifies the contents of a net/http.Request.
// It consumes the body of the request.
// The JSON content can be accessed using FederationRequest.Content()
// Returns an 400 error if there was a problem parsing the request.
// It authenticates the request using an ed25519 signature using the KeyRing.
// The origin server can be accessed using FederationRequest.Origin()
// Returns a 401 error if there was a problem authenticating the request.
// HTTP handlers using this should be careful that they only use the parts of
// the request that have been authenticated: the method, the request path,
// the query parameters, and the JSON content. In particular the version of
// HTTP and the headers aren't protected by the signature.
func VerifyHTTPRequest(
	req *http.Request, now time.Time, destination ServerName, keys KeyRing,
) (*FederationRequest, util.JSONResponse) {
	request, err := readHTTPRequest(req)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Print("Error parsing HTTP headers")
		return nil, util.MessageResponse(400, "Bad Request")
	}
	request.fields.Destination = destination

	// The request fields are already in the form required by the specification
	// So we can just serialise the request fields using the default marshaller
	toVerify, err := json.Marshal(request.fields)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Print("Error parsing JSON")
		return nil, util.MessageResponse(400, "Invalid JSON")
	}

	if request.Origin() == "" {
		message := "Missing \"Authorization: X-Matrix ...\" HTTP header"
		util.GetLogger(req.Context()).WithError(err).Print(message)
		return nil, util.MessageResponse(401, message)
	}

	results, err := keys.VerifyJSONs([]VerifyJSONRequest{{
		ServerName: request.Origin(),
		AtTS:       AsTimestamp(now),
		Message:    toVerify,
	}})
	if err != nil {
		message := "Error authenticating request"
		util.GetLogger(req.Context()).WithError(err).Print(message)
		return nil, util.MessageResponse(500, message)
	}
	if results[0].Error != nil {
		message := "Invalid request signature"
		util.GetLogger(req.Context()).WithError(results[0].Error).Print(message)
		return nil, util.MessageResponse(401, message)
	}

	return request, util.JSONResponse{Code: 200, JSON: struct{}{}}
}

// Returns an error if there was a problem reading the content of the request
func readHTTPRequest(req *http.Request) (*FederationRequest, error) {
	var result FederationRequest

	result.fields.Method = req.Method
	result.fields.RequestURI = req.URL.RequestURI()

	content, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if len(content) != 0 {
		if req.Header.Get("Content-Type") != "application/json" {
			return nil, fmt.Errorf(
				"gomatrixserverlib: The request must be \"application/json\" not %q",
				req.Header.Get("Content-Type"),
			)
		}
		result.fields.Content = rawJSON(content)
	}

	for _, authorization := range req.Header["Authorization"] {
		scheme, origin, key, sig := parseAuthorization(authorization)
		if scheme != "X-Matrix" {
			// Ignore unknown types of Authorization.
			continue
		}
		if origin == "" || key == "" || sig == "" {
			return nil, fmt.Errorf("gomatrixserverlib: invalid X-Matrix authorization header")
		}
		if result.fields.Origin != "" && result.fields.Origin != origin {
			return nil, fmt.Errorf("gomatrixserverlib: different origins in X-Matrix authorization headers")
		}
		result.fields.Origin = origin
		if result.fields.Signatures == nil {
			result.fields.Signatures = map[ServerName]map[KeyID]string{origin: map[KeyID]string{key: sig}}
		} else {
			result.fields.Signatures[origin][key] = sig
		}
	}

	return &result, nil
}

func parseAuthorization(header string) (scheme string, origin ServerName, key KeyID, sig string) {
	parts := strings.SplitN(header, " ", 2)
	scheme = parts[0]
	if scheme != "X-Matrix" {
		return
	}
	if len(parts) != 2 {
		return
	}
	for _, data := range strings.Split(parts[1], ",") {
		pair := strings.SplitN(data, "=", 2)
		if len(pair) != 2 {
			continue
		}
		name := pair[0]
		value := strings.Trim(pair[1], "\"")
		if name == "origin" {
			origin = ServerName(value)
		}
		if name == "key" {
			key = KeyID(value)
		}
		if name == "sig" {
			sig = value
		}
	}
	return
}
