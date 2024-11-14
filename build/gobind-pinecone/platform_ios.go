// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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
