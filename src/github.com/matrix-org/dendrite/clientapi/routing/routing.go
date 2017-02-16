package routing

import (
	"github.com/matrix-org/dendrite/clientapi/readers"
	_ "github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"strings"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Return true if this path should be handled by this handler
type matcher func(path string) bool

type lookup struct {
	Matches matcher
	Handler http.Handler
}

func newLookup(name string, m matcher, h util.JSONRequestHandler) lookup {
	return lookup{
		Matches: m,
		Handler: prometheus.InstrumentHandler(name, util.MakeJSONAPI(h)),
	}
}

func matchesString(str string) func(path string) bool {
	return func(path string) bool {
		return path == str
	}
}

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(mux *http.ServeMux, httpClient *http.Client) {
	var r0lookups []lookup
	r0lookups = append(r0lookups, newLookup("sync", matchesString("/sync"), &readers.Sync{}))

	mux.Handle("/metrics", prometheus.Handler())
	mux.HandleFunc("/api/", func(w http.ResponseWriter, req *http.Request) {
		clientServerPath := strings.TrimPrefix(req.URL.Path, "/api")
		if strings.HasPrefix(clientServerPath, pathPrefixR0) {
			apiPath := strings.TrimPrefix(clientServerPath, pathPrefixR0)
			for _, lookup := range r0lookups {
				if lookup.Matches(apiPath) {
					lookup.Handler.ServeHTTP(w, req)
					return
				}
			}
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"error":"Not found","errcode":"M_NOT_FOUND"}`))
	})
}
