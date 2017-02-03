package util

import "fmt"

// HTTPError An HTTP Error response, which may wrap an underlying native Go Error.
type HTTPError struct {
	WrappedError error
	Message      string
	Code         int
}

func (e HTTPError) Error() string {
	var wrappedErrMsg string
	if e.WrappedError != nil {
		wrappedErrMsg = e.WrappedError.Error()
	}
	return fmt.Sprintf("%s: %d: %s", e.Message, e.Code, wrappedErrMsg)
}
