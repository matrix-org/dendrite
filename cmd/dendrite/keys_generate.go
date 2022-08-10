package main

import (
	"fmt"

	"github.com/matrix-org/dendrite/test"
	"github.com/spf13/cobra"
)

var (
	tlsCertFile       string
	tlsKeyFile        string
	privateKeyFile    string
	authorityCertFile string
	authorityKeyFile  string
	keySize           int
	// already in helper_config.go defined:
	// serverName        string
)

func init() {
	generateKeysCmd.Flags().StringVarP(&tlsCertFile, "tls-cert", "", "", "An X509 certificate file to generate for use for TLS")
	generateKeysCmd.Flags().StringVarP(&tlsKeyFile, "tls-key", "", "", "An RSA private key file to generate for use for TLS")
	generateKeysCmd.MarkFlagsRequiredTogether("tls-cert", "tls-key")

	generateKeysCmd.Flags().StringVarP(&privateKeyFile, "private-key", "", "", "An Ed25519 private key to generate for use for object signing")

	generateKeysCmd.Flags().StringVarP(&authorityCertFile, "tls-authority-cert", "", "", "Optional: Create TLS certificate/keys based on this CA authority. Useful for integration testing.")
	generateKeysCmd.Flags().StringVarP(&authorityKeyFile, "tls-authority-key", "", "", "Optional: Create TLS certificate/keys based on this CA authority. Useful for integration testing.")
	generateKeysCmd.MarkFlagsRequiredTogether("tls-authority-cert", "tls-authority-key")

	generateKeysCmd.Flags().StringVarP(&serverName, "server", "", "", "Optional: Create TLS certificate/keys with this domain name set. Useful for integration testing.")
	generateKeysCmd.Flags().IntVarP(&keySize, "keysize", "", 4096, "Optional: Create TLS RSA private key with the given key size")

	keysCmd.AddCommand(generateKeysCmd)
}

var generateKeysCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate key files which are required by dendrite.",
	Run: func(cmd *cobra.Command, args []string) {
		if tlsCertFile == "" && tlsKeyFile == "" && privateKeyFile == "" {
			fmt.Println("Nothing todo, please set argument.")
			cmd.Usage()
			return
		}
		if tlsKeyFile != "" {
			if authorityKeyFile == "" {
				if err := test.NewTLSKey(tlsKeyFile, tlsCertFile, keySize); err != nil {
					panic(err)
				}
			} else {
				// generate the TLS cert/key based on the authority given.
				if err := test.NewTLSKeyWithAuthority(serverName, tlsKeyFile, tlsCertFile, authorityKeyFile, authorityCertFile, keySize); err != nil {
					panic(err)
				}
			}
			fmt.Printf("Created TLS cert file:    %s\n", tlsCertFile)
			fmt.Printf("Created TLS key file:     %s\n", tlsKeyFile)
		}

		if privateKeyFile != "" {
			if err := test.NewMatrixKey(privateKeyFile); err != nil {
				panic(err)
			}
			fmt.Printf("Created private key file: %s\n", privateKeyFile)
		}
	},
}
