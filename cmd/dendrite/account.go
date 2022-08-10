package main

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(accountCmd)
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage Account on this server",
}
