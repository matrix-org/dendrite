package main

import (
	"fmt"
	"os"

	"github.com/matrix-org/dendrite/internal"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dendrite",
	Short: "Dendrite is a second-generation Matrix homeserver written in Go! ",
	Long: `Dendrite is a second-generation Matrix homeserver written in Go.
It intends to provide an efficient, reliable and scalable alternative to Synapse.`,
	Version: internal.VersionString(),
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
