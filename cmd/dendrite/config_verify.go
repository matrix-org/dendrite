package main

import (
	"fmt"
	"os"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/structs"
	"github.com/spf13/cobra"
)

func init() {
	verifyConfigCmd.Flags().StringVarP(&configPath, "config", "c", "", "write config to file if not set to print in yaml")
	verifyConfigCmd.Flags().BoolVarP(&monolithic, "monolithic", "", true, "")

	configCmd.AddCommand(verifyConfigCmd)
}

var verifyConfigCmd = &cobra.Command{
	Use:   "verify",
	Short: "verify configuration",
	Long:  "verify configuration - read file+env and print result",
	Run: func(cmd *cobra.Command, args []string) {
		k := koanf.New("/")
		readConfig(false, monolithic, configPath, "./")

		k.Load(structs.Provider(cfg, "config"), nil)

		b, err := k.Marshal(yaml.Parser())
		if err != nil {
			fmt.Println("unable to generate yaml", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	},
}
