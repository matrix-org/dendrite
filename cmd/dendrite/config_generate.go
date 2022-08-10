package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/structs"
	"github.com/spf13/cobra"
)

var (
	dirPath = "./"
)

func init() {
	generateConfigCmd.Flags().StringVarP(&configPath, "config", "c", "", "write config to file if not set to print in yaml")

	generateConfigCmd.Flags().StringVarP(&serverName, "server", "", "", "The domain name of the server if not 'localhost'")
	generateConfigCmd.Flags().StringVarP(&dbURI, "db", "", "", "The DB URI to use for all components if not SQLite files")
	generateConfigCmd.Flags().StringVarP(&dirPath, "dir", "", "./", "The folder to use for paths (like SQLite databases, media storage)")

	generateConfigCmd.Flags().BoolVarP(&defaultsForCI, "ci", "", false, "Populate the configuration with sane defaults for use in CI")
	generateConfigCmd.Flags().BoolVarP(&monolithic, "monolithic", "", true, "Generate a config that makes sense for monolith deployments")

	configCmd.AddCommand(generateConfigCmd)
}

var generateConfigCmd = &cobra.Command{
	Use:   "generate",
	Short: "generate configuration file",
	Long:  "generate default configuration (overwrite is possible by commandline arguments and enviroment variables",
	Run: func(cmd *cobra.Command, args []string) {
		k := koanf.New("/")
		// "" <- do not read file - we like to write
		readConfig(true, monolithic, "", dirPath)

		k.Load(structs.Provider(cfg, "config"), nil)

		var parser koanf.Parser
		if configPath != "" {
			var fileExt string
			parser, fileExt = getParser(configPath)
			if parser == nil {
				fmt.Println("unsupported file extention:", fileExt)
				os.Exit(1)
			}
		} else {
			parser = yaml.Parser()
		}

		b, err := k.Marshal(parser)
		if err != nil {
			fmt.Println("unable to generate yaml", err)
			os.Exit(1)
		}
		if configPath != "" {
			ioutil.WriteFile(configPath, b, 0644)
			fmt.Println("config file saved in:", configPath)
		} else {
			fmt.Println(string(b))
		}
	},
}
