// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/matrix-org/dendrite/test"
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
