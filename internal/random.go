package internal

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateBlob(blobLen int) (string, error) {
	b := make([]byte, blobLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}
