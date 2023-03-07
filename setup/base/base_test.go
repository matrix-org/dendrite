package base_test

import (
	"bytes"
	"context"
	"embed"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/stretchr/testify/assert"
)

//go:embed static/*.gotmpl
var staticContent embed.FS

func TestLandingPage_Tcp(t *testing.T) {
	// generate the expected result
	tmpl := template.Must(template.ParseFS(staticContent, "static/*.gotmpl"))
	expectedRes := &bytes.Buffer{}
	err := tmpl.ExecuteTemplate(expectedRes, "index.gotmpl", map[string]string{
		"Version": internal.VersionString(),
	})
	assert.NoError(t, err)

	b, _, _ := testrig.Base(nil)
	defer b.Close()

	// hack: create a server and close it immediately, just to get a random port assigned
	s := httptest.NewServer(nil)
	s.Close()

	// start base with the listener and wait for it to be started
	address, err := config.HTTPAddress(s.URL)
	assert.NoError(t, err)
	go b.SetupAndServeHTTP(address, nil, nil)
	time.Sleep(time.Millisecond * 10)

	// When hitting /, we should be redirected to /_matrix/static, which should contain the landing page
	req, err := http.NewRequest(http.MethodGet, s.URL, nil)
	assert.NoError(t, err)

	// do the request
	resp, err := s.Client().Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// read the response
	buf := &bytes.Buffer{}
	_, err = buf.ReadFrom(resp.Body)
	assert.NoError(t, err)

	// Using .String() for user friendly output
	assert.Equal(t, expectedRes.String(), buf.String(), "response mismatch")
}

func TestLandingPage_UnixSocket(t *testing.T) {
	// generate the expected result
	tmpl := template.Must(template.ParseFS(staticContent, "static/*.gotmpl"))
	expectedRes := &bytes.Buffer{}
	err := tmpl.ExecuteTemplate(expectedRes, "index.gotmpl", map[string]string{
		"Version": internal.VersionString(),
	})
	assert.NoError(t, err)

	b, _, _ := testrig.Base(nil)
	defer b.Close()

	tempDir := t.TempDir()
	socket := path.Join(tempDir, "socket")
	// start base with the listener and wait for it to be started
	address := config.UnixSocketAddress(socket, 0755)
	assert.NoError(t, err)
	go b.SetupAndServeHTTP(address, nil, nil)
	time.Sleep(time.Millisecond * 100)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socket)
			},
		},
	}
	resp, err := client.Get("http://unix/")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// read the response
	buf := &bytes.Buffer{}
	_, err = buf.ReadFrom(resp.Body)
	assert.NoError(t, err)

	// Using .String() for user friendly output
	assert.Equal(t, expectedRes.String(), buf.String(), "response mismatch")
}
