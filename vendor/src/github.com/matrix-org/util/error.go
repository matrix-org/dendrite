package util

import "fmt"

// HTTPError An HTTP Error response, which may wrap an underlying native Go Error.
type HTTPError struct {
	WrappedError error
	// A human-readable message to return to the client in a JSON response. This
	// is ignored if JSON is supplied.
	Message string
	// HTTP status code.
	Code int
	// JSON represents the JSON that should be serialized and sent to the client
	// instead of the given Message.
	JSON interface{}
}

func (e HTTPError) Error() string {
	var wrappedErrMsg string
	if e.WrappedError != nil {
		wrappedErrMsg = e.WrappedError.Error()
	}
	return fmt.Sprintf("%s: %d: %s", e.Message, e.Code, wrappedErrMsg)
}
