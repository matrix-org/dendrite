package main

import (
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/readers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

// setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func setup(mux *http.ServeMux, httpClient *http.Client) {
	mux.Handle("/metrics", prometheus.Handler())
	mux.Handle("/api/send", prometheus.InstrumentHandler("send_message", util.MakeJSONAPI(&writers.SendMessage{})))
	mux.Handle("/api/sync", prometheus.InstrumentHandler("sync", util.MakeJSONAPI(&readers.Sync{})))
}

func main() {
	bindAddr := os.Getenv("BIND_ADDRESS")
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	log.Info("Starting clientapi")
	setup(http.DefaultServeMux, http.DefaultClient)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
