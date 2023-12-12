package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_dispositionFor(t *testing.T) {
	assert.Equal(t, "attachment", contentDispositionFor(""), "empty content type")
	assert.Equal(t, "attachment", contentDispositionFor("image/svg"), "image/svg")
	assert.Equal(t, "inline", contentDispositionFor("image/jpeg"), "image/jpg")
}
