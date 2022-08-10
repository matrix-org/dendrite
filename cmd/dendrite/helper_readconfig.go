package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/spf13/cobra"
)

var (
	configPath string
	monolithic bool

	enableRegistrationWithoutVerification = true
)

func readConfigCmd(monolithic bool) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		readConfig(false, monolithic, configPath, "./")
	}
}

func readConfig(generate bool, monolithic bool, configPath, dirPath string) {
	configDefaults(generate, monolithic, dirPath)

	k := koanf.New("/")

	if configPath != "" {
		parser, fileExt := getParser(configPath)
		if parser == nil {
			fmt.Println("unsupported file extention:", fileExt)
			os.Exit(1)
		}
		if err := k.Load(file.Provider(configPath), parser); err != nil {
			fmt.Println("read file config:", err)
			os.Exit(1)
		}
	}

	if err := k.Load(env.Provider("DENDRITE_", "/", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "DENDRITE_")), "__", "/", -1)
	}), nil); err != nil {
		fmt.Println("read env config:", err)
		os.Exit(1)
	}

	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "config"}); err != nil {
		fmt.Println("Can't unmarshal config:", err)
		os.Exit(1)
	}

	basePath, err := filepath.Abs(".")
	if err != nil {
		fmt.Println("Can't get working dir, for read files allocated by config:", err)
		os.Exit(1)
	}

	if err := cfg.Load(basePath, monolithic); err != nil {
		fmt.Println("Can't setup config:", err)
		os.Exit(1)
	}

	cfg.ClientAPI.OpenRegistrationWithoutVerificationEnabled = enableRegistrationWithoutVerification
}
