package routing

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/element-hq/dendrite/mediaapi/types"
	"github.com/stretchr/testify/assert"
)

func Test_dispositionFor(t *testing.T) {
	assert.Equal(t, "attachment", contentDispositionFor(""), "empty content type")
	assert.Equal(t, "attachment", contentDispositionFor("image/svg"), "image/svg")
	assert.Equal(t, "inline", contentDispositionFor("image/jpeg"), "image/jpg")
}

func Test_Multipart(t *testing.T) {
	r := &downloadRequest{
		MediaMetadata: &types.MediaMetadata{},
	}
	data := bytes.Buffer{}
	responseBody := "This media is plain text. Maybe somebody used it as a paste bin."
	data.WriteString(responseBody)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := multipartResponse(w, r, "text/plain", &data)
		assert.NoError(t, err)
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	assert.NoError(t, err)
	defer resp.Body.Close()
	// contentLength is always 0, since there's no Content-Length header on the multipart part.
	_, reader, err := parseMultipartResponse(r, resp, 1000)
	assert.NoError(t, err)
	gotResponse, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, responseBody, string(gotResponse))
}
