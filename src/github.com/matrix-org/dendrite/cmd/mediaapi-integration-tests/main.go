// Copyright 2017 Vector Creations Ltd
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

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/common/test"
)

var (
	// How long to wait for the server to write the expected output messages.
	// This needs to be high enough to account for the time it takes to create
	// the postgres database tables which can take a while on travis.
	timeoutString = test.Defaulting(os.Getenv("TIMEOUT"), "10s")
	// The name of maintenance database to connect to in order to create the test database.
	postgresDatabase = test.Defaulting(os.Getenv("POSTGRES_DATABASE"), "postgres")
	// The name of the test database to create.
	testDatabaseName = test.Defaulting(os.Getenv("DATABASE_NAME"), "mediaapi_test")
	// Postgres docker container name (for running psql)
	postgresContainerName = os.Getenv("POSTGRES_CONTAINER")
)

var thumbnailPregenerationConfig = (`
thumbnail_sizes:
- width: 32
  height: 32
  method: crop
- width: 96
  height: 96
  method: crop
- width: 320
  height: 240
  method: scale
- width: 640
  height: 480
  method: scale
- width: 800
  height: 600
  method: scale
`)

const serverType = "media-api"

var testDatabaseTemplate = "dbname=%s sslmode=disable binary_parameters=yes"

var timeout time.Duration

func startMediaAPI(suffix string, dynamicThumbnails bool) (*exec.Cmd, chan error, string) {
	dir, err := ioutil.TempDir("", serverType+"-server-test"+suffix)
	if err != nil {
		panic(err)
	}

	configFilename := serverType + "-server-test-config" + suffix + ".yaml"
	configFileContents := makeConfig(suffix, dir, dynamicThumbnails)

	serverArgs := []string{
		"--config", configFilename,
		"--listen", "localhost:1777" + suffix,
	}

	databases := []string{
		testDatabaseName + suffix,
	}

	cmd, cmdChan := test.StartServer(
		serverType,
		serverArgs,
		suffix,
		configFilename,
		configFileContents,
		postgresDatabase,
		postgresContainerName,
		databases,
	)
	return cmd, cmdChan, dir
}

func makeConfig(suffix, basePath string, dynamicThumbnails bool) string {
	return fmt.Sprintf(
		`
server_name: "%s"
base_path: %s
max_file_size_bytes: %s
database: "%s"
dynamic_thumbnails: %s
%s`,
		"localhost:1777"+suffix,
		basePath,
		"10485760",
		fmt.Sprintf(testDatabaseTemplate, testDatabaseName+suffix),
		strconv.FormatBool(dynamicThumbnails),
		thumbnailPregenerationConfig,
	)
}

func cleanUpServer(cmd *exec.Cmd, dir string) {
	cmd.Process.Kill() // ensure server is dead, only cleaning up so don't care about errors this returns.
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("WARNING: Failed to remove temporary directory %v: %q\n", dir, err)
	}
}

// Runs a battery of media API server tests
// The tests will pause at various points in this list to conduct tests on the HTTP responses before continuing.
func main() {
	fmt.Println("==TESTING==", os.Args[0])

	var err error
	timeout, err = time.ParseDuration(timeoutString)
	if err != nil {
		fmt.Printf("ERROR: Invalid timeout string %v: %q\n", timeoutString, err)
		return
	}

	// create server1 with only pre-generated thumbnails allowed
	server1Cmd, server1CmdChan, server1Dir := startMediaAPI("1", false)
	defer cleanUpServer(server1Cmd, server1Dir)
	testDownload("localhost:17771", "localhost:17771", "doesnotexist", "", 404, server1CmdChan)

	// create server2 with dynamic thumbnail generation
	server2Cmd, server2CmdChan, server2Dir := startMediaAPI("2", true)
	defer cleanUpServer(server2Cmd, server2Dir)
	testDownload("localhost:17772", "localhost:17772", "doesnotexist", "", 404, server2CmdChan)
}

func getMediaURI(scheme, host, endpoint string, components []string) string {
	pathComponents := []string{host, "api/_matrix/media/v1", endpoint}
	pathComponents = append(pathComponents, components...)
	return scheme + path.Join(pathComponents...)
}

func testDownload(host, origin, mediaID, wantedBody string, wantedStatusCode int, serverCmdChan chan error) {
	req, err := http.NewRequest(
		"GET",
		getMediaURI("http://", host, "download", []string{
			origin,
			mediaID,
		}),
		nil,
	)
	if err != nil {
		panic(err)
	}
	testReq := &test.Request{
		Req:              req,
		WantedStatusCode: wantedStatusCode,
		WantedBody:       wantedBody,
	}
	testReq.Run("media-api", timeout, serverCmdChan)
}
