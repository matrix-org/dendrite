package base

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// noOpHTTPTransport is used to disable federation.
var noOpHTTPTransport = &http.Transport{
	Dial: func(_, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("federation prohibited by configuration")
	},
	DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("federation prohibited by configuration")
	},
	DialTLS: func(_, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("federation prohibited by configuration")
	},
}

func init() {
	noOpHTTPTransport.RegisterProtocol("matrix", &noOpHTTPRoundTripper{})
}

type noOpHTTPRoundTripper struct {
}

func (y *noOpHTTPRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("federation prohibited by configuration")
}
