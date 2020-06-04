package sqlutil

import (
	"fmt"
	"net/url"
)

// ParseFileURI returns the filepath in the given file: URI. Specifically, this will handle
// both relative (file:foo.db) and absolute (file:///path/to/foo) paths.
func ParseFileURI(dataSourceName string) (string, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return "", err
	}
	var cs string
	if uri.Opaque != "" { // file:filename.db
		cs = uri.Opaque
	} else if uri.Path != "" { // file:///path/to/filename.db
		cs = uri.Path
	} else {
		return "", fmt.Errorf("invalid file uri: %s", dataSourceName)
	}
	return cs, nil
}
