package routing

import (
	"bytes"
	"io"
	"testing"

	"github.com/patrickmn/go-cache"
)

func TestSharedSecretRegister(t *testing.T) {
	// these values have come from a local synapse instance to ensure compatibility
	jsonStr := []byte(`{"admin":false,"mac":"f1ba8d37123866fd659b40de4bad9b0f8965c565","nonce":"759f047f312b99ff428b21d581256f8592b8976e58bc1b543972dc6147e529a79657605b52d7becd160ff5137f3de11975684319187e06901955f79e5a6c5a79","password":"wonderland","username":"alice"}`)
	sharedSecret := "dendritetest"

	req, err := NewSharedSecretRegistrationRequest(io.NopCloser(bytes.NewBuffer(jsonStr)))
	if err != nil {
		t.Fatalf("failed to read request: %s", err)
	}

	r := NewSharedSecretRegistration(sharedSecret)

	// force the nonce to be known
	r.nonces.Set(req.Nonce, true, cache.DefaultExpiration)

	valid, err := r.IsValidMacLogin(req.Nonce, req.User, req.Password, req.Admin, req.MacBytes)
	if err != nil {
		t.Fatalf("failed to check for valid mac: %s", err)
	}
	if !valid {
		t.Errorf("mac login failed, wanted success")
	}

	// modify the mac so it fails
	req.MacBytes[0] = 0xff
	valid, err = r.IsValidMacLogin(req.Nonce, req.User, req.Password, req.Admin, req.MacBytes)
	if err != nil {
		t.Fatalf("failed to check for valid mac: %s", err)
	}
	if valid {
		t.Errorf("mac login succeeded, wanted failure")
	}
}
