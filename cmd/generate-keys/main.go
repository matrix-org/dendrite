// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/element-hq/dendrite/test"
)

const usage = `Usage: %s

Generate key files which are required by dendrite.

Arguments:

`

var (
	tlsCertFile       = flag.String("tls-cert", "", "An X509 certificate file to generate for use for TLS")
	tlsKeyFile        = flag.String("tls-key", "", "An RSA private key file to generate for use for TLS")
	privateKeyFile    = flag.String("private-key", "", "An Ed25519 private key to generate for use for object signing")
	authorityCertFile = flag.String("tls-authority-cert", "", "Optional: Create TLS certificate/keys based on this CA authority. Useful for integration testing.")
	authorityKeyFile  = flag.String("tls-authority-key", "", "Optional: Create TLS certificate/keys based on this CA authority. Useful for integration testing.")
	serverName        = flag.String("server", "", "Optional: Create TLS certificate/keys with this domain name set. Useful for integration testing.")
	keySize           = flag.Int("keysize", 4096, "Optional: Create TLS RSA private key with the given key size")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *tlsCertFile == "" && *tlsKeyFile == "" && *privateKeyFile == "" {
		flag.Usage()
		return
	}

	if *tlsCertFile != "" || *tlsKeyFile != "" {
		if *tlsCertFile == "" || *tlsKeyFile == "" {
			log.Fatal("Zero or both of --tls-key and --tls-cert must be supplied")
		}
		if *authorityCertFile == "" && *authorityKeyFile == "" {
			if err := test.NewTLSKey(*tlsKeyFile, *tlsCertFile, *keySize); err != nil {
				panic(err)
			}
		} else {
			// generate the TLS cert/key based on the authority given.
			if err := test.NewTLSKeyWithAuthority(*serverName, *tlsKeyFile, *tlsCertFile, *authorityKeyFile, *authorityCertFile, *keySize); err != nil {
				panic(err)
			}
		}
		fmt.Printf("Created TLS cert file:    %s\n", *tlsCertFile)
		fmt.Printf("Created TLS key file:     %s\n", *tlsKeyFile)
	}

	if *privateKeyFile != "" {
		if err := test.NewMatrixKey(*privateKeyFile); err != nil {
			panic(err)
		}
		fmt.Printf("Created private key file: %s\n", *privateKeyFile)
	}
}
