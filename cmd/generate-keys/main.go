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

	"github.com/matrix-org/dendrite/common/basecomponent"

	"github.com/matrix-org/dendrite/common/test"
)

const usage = `Usage: %s

Generate key files which are required by dendrite.

Arguments:

`

var (
	tlsCertFile    = flag.String("tls-cert", basecomponent.EnvParse("DENDRITE_GENKEYS_TLS_CERT_OUT", ""), "An X509 certificate file to generate for use for TLS")
	tlsKeyFile     = flag.String("tls-key", basecomponent.EnvParse("DENDRITE_GENKEYS_TLS_KEY_OUT", ""), "An RSA private key file to generate for use for TLS")
	privateKeyFile = flag.String("private-key", basecomponent.EnvParse("DENDRITE_GENKEYS_PEM_OUT", ""), "An Ed25519 private key to generate for use for object signing")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *tlsCertFile != "" || *tlsKeyFile != "" {
		if *tlsCertFile == "" || *tlsKeyFile == "" {
			log.Fatal("Zero or both of --tls-key and --tls-cert must be supplied")
		}
		if err := test.NewTLSKey(*tlsKeyFile, *tlsCertFile); err != nil {
			panic(err)
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
