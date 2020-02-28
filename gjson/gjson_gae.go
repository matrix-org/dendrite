//+build appengine js

package gjson

import (
	"reflect"
	"unsafe"
)

func getBytes(json []byte, path string) Result {
	return Get(string(json), path)
}

// fillIndex finds the position of Raw data and assigns it to the Index field
// of the resulting value. If the position cannot be found then Index zero is
// used instead.
func fillIndex(json string, c *parseContext) {
	if len(c.value.Raw) > 0 && !c.calcd {
		jhdr := *(*reflect.StringHeader)(unsafe.Pointer(&json))
		rhdr := *(*reflect.StringHeader)(unsafe.Pointer(&(c.value.Raw)))
		c.value.Index = int(rhdr.Data - jhdr.Data)
		if c.value.Index < 0 || c.value.Index >= len(json) {
			c.value.Index = 0
		}
	}
}

func stringBytes(s string) []byte {
	return []byte(s)
}

func bytesString(b []byte) string {
	return string(b)
}
