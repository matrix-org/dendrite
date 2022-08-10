package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(keysCmd)
}

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "key tools",
}
