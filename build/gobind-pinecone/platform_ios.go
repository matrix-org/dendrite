//go:build ios
// +build ios

package gobind

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
void Log(const char *text) {
  NSString *nss = [NSString stringWithUTF8String:text];
  NSLog(@"%@", nss);
}
*/
import "C"
import "unsafe"

type BindLogger struct {
}

func (nsl BindLogger) Write(p []byte) (n int, err error) {
	p = append(p, 0)
	cstr := (*C.char)(unsafe.Pointer(&p[0]))
	C.Log(cstr)
	return len(p), nil
}
