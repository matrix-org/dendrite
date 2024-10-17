// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

//go:build !wasm
// +build !wasm

package main

import "fmt"

func main() {
	fmt.Println("dendritejs: no-op when not compiling for WebAssembly")
}
