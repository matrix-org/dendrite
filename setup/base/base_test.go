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

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/httputil"
	basepkg "github.com/element-hq/dendrite/setup/base"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
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

	processCtx := process.NewProcessContext()
	routers := httputil.NewRouters()
	cfg := config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{Generate: true, SingleDatabase: true})

	// hack: create a server and close it immediately, just to get a random port assigned
	s := httptest.NewServer(nil)
	s.Close()

	// start base with the listener and wait for it to be started
	address, err := config.HTTPAddress(s.URL)
	assert.NoError(t, err)
	go basepkg.SetupAndServeHTTP(processCtx, &cfg, routers, address, nil, nil)
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

	processCtx := process.NewProcessContext()
	routers := httputil.NewRouters()
	cfg := config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{Generate: true, SingleDatabase: true})

	tempDir := t.TempDir()
	socket := path.Join(tempDir, "socket")
	// start base with the listener and wait for it to be started
	address, err := config.UnixSocketAddress(socket, "755")
	assert.NoError(t, err)
	go basepkg.SetupAndServeHTTP(processCtx, &cfg, routers, address, nil, nil)
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
