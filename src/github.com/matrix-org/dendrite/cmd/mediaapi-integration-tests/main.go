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
	"path/filepath"
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
	// Test image to be uploaded/downloaded
	testJPEG = test.Defaulting(os.Getenv("TEST_JPEG_PATH"), "src/github.com/matrix-org/dendrite/cmd/mediaapi-integration-tests/totem.jpg")
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

func startMediaAPI(suffix string, dynamicThumbnails bool) (*exec.Cmd, chan error, string, *exec.Cmd, chan error, string, string) {
	dir, err := ioutil.TempDir("", serverType+"-server-test"+suffix)
	if err != nil {
		panic(err)
	}

	serverAddr := "localhost:177" + suffix + "9"
	proxyAddr := "localhost:1800" + suffix

	configFilename := serverType + "-server-test-config" + suffix + ".yaml"
	configFileContents := makeConfig(proxyAddr, suffix, dir, dynamicThumbnails)

	serverArgs := []string{
		"--config", configFilename,
		"--listen", serverAddr,
	}

	databases := []string{
		testDatabaseName + suffix,
	}

	proxyCmd, proxyCmdChan := test.StartProxy(
		proxyAddr,
		"http://localhost:177"+suffix+"6",
		"http://localhost:177"+suffix+"8",
		"http://"+serverAddr,
	)

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

	fmt.Printf("==TESTSERVER== STARTED %v -> %v : %v\n", proxyAddr, serverAddr, dir)
	return cmd, cmdChan, serverAddr, proxyCmd, proxyCmdChan, proxyAddr, dir
}

func makeConfig(serverAddr, suffix, basePath string, dynamicThumbnails bool) string {
	return fmt.Sprintf(
		`
server_name: "%s"
base_path: %s
max_file_size_bytes: %s
database: "%s"
dynamic_thumbnails: %s
%s`,
		serverAddr,
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
	server1Cmd, server1CmdChan, _, server1ProxyCmd, _, server1ProxyAddr, server1Dir := startMediaAPI("1", false)
	defer cleanUpServer(server1Cmd, server1Dir)
	defer server1ProxyCmd.Process.Kill()
	testDownload(server1ProxyAddr, server1ProxyAddr, "doesnotexist", "", 404, server1CmdChan)

	// upload a JPEG file
	testUpload(server1ProxyAddr, testJPEG, "image/jpeg", `{
		"content_uri": "mxc://localhost:18001/1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0"
	}`, 200, server1CmdChan)

	// download that JPEG file
	testDownload(server1ProxyAddr, "localhost:18001", "1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0", "", 200, server1CmdChan)

	// thumbnail that JPEG file
	testThumbnail(64, 64, "crop", server1ProxyAddr, "localhost:18001", "1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0", "", 200, server1CmdChan)

	// create server2 with dynamic thumbnail generation
	server2Cmd, server2CmdChan, _, server2ProxyCmd, _, server2ProxyAddr, server2Dir := startMediaAPI("2", true)
	defer cleanUpServer(server2Cmd, server2Dir)
	defer server2ProxyCmd.Process.Kill()
	testDownload(server2ProxyAddr, server2ProxyAddr, "doesnotexist", "", 404, server2CmdChan)

	// download that JPEG file via server2
	testDownload(server2ProxyAddr, "localhost:18001", "1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0", "", 200, server2CmdChan)

	// thumbnail that JPEG file via server2
	testThumbnail(800, 600, "scale", server2ProxyAddr, "localhost:18001", "1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0", "", 200, server2CmdChan)

	// thumbnail that JPEG file via server2
	testThumbnail(10000, 10000, "scale", server2ProxyAddr, "localhost:18001", "1VuVy8u_hmDllD8BrcY0deM34Bl7SPJeY9J6BkMmpx0", "", 200, server2CmdChan)

}

func getMediaURI(scheme, host, endpoint, query string, components []string) string {
	pathComponents := []string{host, "_matrix/media/v1", endpoint}
	pathComponents = append(pathComponents, components...)
	return scheme + path.Join(pathComponents...) + query
}

func testUpload(host, filePath, contentType, wantedBody string, wantedStatusCode int, serverCmdChan chan error) {
	fmt.Printf("==TESTING== upload %v to %v\n", filePath, host)
	file, err := os.Open(filePath)
	defer file.Close()
	if err != nil {
		panic(err)
	}
	filename := filepath.Base(filePath)
	stat, err := file.Stat()
	if os.IsNotExist(err) {
		panic(err)
	}
	fileSize := stat.Size()

	req, err := http.NewRequest(
		"POST",
		getMediaURI("https://", host, "upload", "?filename="+filename, nil),
		file,
	)
	if err != nil {
		panic(err)
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Type", contentType)

	testReq := &test.Request{
		Req:              req,
		WantedStatusCode: wantedStatusCode,
		WantedBody:       test.CanonicalJSONInput([]string{wantedBody})[0],
	}
	if err := testReq.Do(); err != nil {
		panic(err)
	}
	fmt.Printf("==TESTING== upload %v to %v PASSED\n", filePath, host)
}

func testDownload(host, origin, mediaID, wantedBody string, wantedStatusCode int, serverCmdChan chan error) {
	req, err := http.NewRequest(
		"GET",
		getMediaURI("https://", host, "download", "", []string{
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
		WantedBody:       test.CanonicalJSONInput([]string{wantedBody})[0],
	}
	testReq.Run(fmt.Sprintf("download mxc://%v/%v from %v", origin, mediaID, host), timeout, serverCmdChan)
}

func testThumbnail(width, height int, resizeMethod, host, origin, mediaID, wantedBody string, wantedStatusCode int, serverCmdChan chan error) {
	query := fmt.Sprintf("?width=%v&height=%v", width, height)
	if resizeMethod != "" {
		query += "&method=" + resizeMethod
	}
	req, err := http.NewRequest(
		"GET",
		getMediaURI("https://", host, "thumbnail", query, []string{
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
		WantedBody:       test.CanonicalJSONInput([]string{wantedBody})[0],
	}
	testReq.Run(fmt.Sprintf("thumbnail mxc://%v/%v from %v", origin, mediaID, host), timeout, serverCmdChan)
}
