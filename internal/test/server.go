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

package test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/matrix-org/dendrite/internal/config"
)

// Defaulting allows assignment of string variables with a fallback default value
// Useful for use with os.Getenv() for example
func Defaulting(value, defaultValue string) string {
	if value == "" {
		value = defaultValue
	}
	return value
}

// CreateDatabase creates a new database, dropping it first if it exists
func CreateDatabase(command string, args []string, database string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin = strings.NewReader(
		fmt.Sprintf("DROP DATABASE IF EXISTS %s; CREATE DATABASE %s;", database, database),
	)
	// Send stdout and stderr to our stderr so that we see error messages from
	// the psql process
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CreateBackgroundCommand creates an executable command
// The Cmd being executed is returned. A channel is also returned,
// which will have any termination errors sent down it, followed immediately by the channel being closed.
func CreateBackgroundCommand(command string, args []string) (*exec.Cmd, chan error) {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	if err := cmd.Start(); err != nil {
		panic("failed to start server: " + err.Error())
	}
	cmdChan := make(chan error, 1)
	go func() {
		cmdChan <- cmd.Wait()
		close(cmdChan)
	}()
	return cmd, cmdChan
}

// InitDatabase creates the database and config file needed for the server to run
func InitDatabase(postgresDatabase, postgresContainerName string, databases []string) {
	if len(databases) > 0 {
		var dbCmd string
		var dbArgs []string
		if postgresContainerName == "" {
			dbCmd = "psql"
			dbArgs = []string{postgresDatabase}
		} else {
			dbCmd = "docker"
			dbArgs = []string{
				"exec", "-i", postgresContainerName, "psql", "-U", "postgres", postgresDatabase,
			}
		}
		for _, database := range databases {
			if err := CreateDatabase(dbCmd, dbArgs, database); err != nil {
				panic(err)
			}
		}
	}
}

// StartProxy creates a reverse proxy
func StartProxy(bindAddr string, cfg *config.Dendrite) (*exec.Cmd, chan error) {
	proxyArgs := []string{
		"--bind-address", bindAddr,
		"--sync-api-server-url", "http://" + string(cfg.Listen.SyncAPI),
		"--client-api-server-url", "http://" + string(cfg.Listen.ClientAPI),
		"--media-api-server-url", "http://" + string(cfg.Listen.MediaAPI),
		"--tls-cert", "server.crt",
		"--tls-key", "server.key",
	}
	return CreateBackgroundCommand(
		filepath.Join(filepath.Dir(os.Args[0]), "client-api-proxy"),
		proxyArgs,
	)
}

// ListenAndServe will listen on a random high-numbered port and attach the given router.
// Returns the base URL to send requests to. Call `cancel` to shutdown the server, which will block until it has closed.
func ListenAndServe(t *testing.T, router http.Handler, useTLS bool) (apiURL string, cancel func()) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %s", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	srv := http.Server{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.Handler = router
		var err error
		if useTLS {
			certFile := filepath.Join(os.TempDir(), "dendrite.cert")
			keyFile := filepath.Join(os.TempDir(), "dendrite.key")
			err = NewTLSKey(keyFile, certFile)
			if err != nil {
				t.Logf("failed to generate tls key/cert: %s", err)
				return
			}
			err = srv.ServeTLS(listener, certFile, keyFile)
		} else {
			err = srv.Serve(listener)
		}
		if err != nil && err != http.ErrServerClosed {
			t.Logf("Listen failed: %s", err)
		}
	}()

	secure := ""
	if useTLS {
		secure = "s"
	}
	return fmt.Sprintf("http%s://localhost:%d", secure, port), func() {
		_ = srv.Shutdown(context.Background())
		wg.Wait()
	}
}
