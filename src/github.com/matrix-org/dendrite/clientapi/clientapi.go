package main

import (
	"github.com/matrix-org/dendrite/clientapi/routing"
	"net/http"
	"os"

	log "github.com/Sirupsen/logrus"
)

func main() {
	bindAddr := os.Getenv("BIND_ADDRESS")
	if bindAddr == "" {
		log.Panic("No BIND_ADDRESS environment variable found.")
	}
	log.Info("Starting clientapi")
	routing.Setup(http.DefaultServeMux, http.DefaultClient)
	log.Fatal(http.ListenAndServe(bindAddr, nil))
}
