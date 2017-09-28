// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tchannel

import (
	"io"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorMetricKeys(t *testing.T) {
	codes := []SystemErrCode{
		ErrCodeInvalid,
		ErrCodeTimeout,
		ErrCodeCancelled,
		ErrCodeBusy,
		ErrCodeDeclined,
		ErrCodeUnexpected,
		ErrCodeBadRequest,
		ErrCodeNetwork,
		ErrCodeProtocol,
	}

	// Metrics keys should be all lowercase letters and dashes. No spaces,
	// underscores, or other characters.
	expected := regexp.MustCompile(`^[[:lower:]-]+$`)
	for _, c := range codes {
		assert.True(t, expected.MatchString(c.MetricsKey()), "Expected metrics key for code %s to be well-formed.", c.String())
	}

	// Unexpected codes may have poorly-formed keys.
	assert.Equal(t, "SystemErrCode(13)", SystemErrCode(13).MetricsKey(), "Expected invalid error codes to use a fallback metrics key format.")
}

func TestInvalidError(t *testing.T) {
	code := GetSystemErrorCode(nil)
	assert.Equal(t, ErrCodeInvalid, code, "nil error should produce ErrCodeInvalid")
}

func TestUnexpectedError(t *testing.T) {
	code := GetSystemErrorCode(io.EOF)
	assert.Equal(t, ErrCodeUnexpected, code, "non-tchannel SystemError should produce ErrCodeUnexpected")
}

func TestSystemError(t *testing.T) {
	code := GetSystemErrorCode(ErrTimeout)
	assert.Equal(t, ErrCodeTimeout, code, "tchannel timeout error produces ErrCodeTimeout")
}

func TestRelayMetricsKey(t *testing.T) {
	for i := 0; i <= 256; i++ {
		code := SystemErrCode(i)
		assert.Equal(t, "relay-"+code.MetricsKey(), code.relayMetricsKey(), "Unexpected relay metrics key for %v", code)
	}
}
