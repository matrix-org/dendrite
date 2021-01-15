package main

import (
	"flag"
	"fmt"

	"github.com/matrix-org/dendrite/setup/config"
	"gopkg.in/yaml.v2"
)

func main() {
	defaultsForCI := flag.Bool("ci", false, "sane defaults for CI testing")
	flag.Parse()

	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.TrustedIDServers = []string{
		"matrix.org",
		"vector.im",
	}
	cfg.Logging = []config.LogrusHook{
		{
			Type:  "file",
			Level: "info",
			Params: map[string]interface{}{
				"path": "/var/log/dendrite",
			},
		},
	}
	cfg.SigningKeyServer.KeyPerspectives = config.KeyPerspectives{
		{
			ServerName: "matrix.org",
			Keys: []config.KeyPerspectiveTrustKey{
				{
					KeyID:     "ed25519:auto",
					PublicKey: "Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw",
				},
				{
					KeyID:     "ed25519:a_RXGa",
					PublicKey: "l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ",
				},
			},
		},
	}
	cfg.MediaAPI.ThumbnailSizes = []config.ThumbnailSize{
		{
			Width:        32,
			Height:       32,
			ResizeMethod: "crop",
		},
		{
			Width:        96,
			Height:       96,
			ResizeMethod: "crop",
		},
		{
			Width:        640,
			Height:       480,
			ResizeMethod: "scale",
		},
	}

	if *defaultsForCI {
		cfg.ClientAPI.RateLimiting.Enabled = false
		cfg.FederationSender.DisableTLSValidation = true
		cfg.MSCs.MSCs = []string{"msc2836","msc2946"}
		cfg.Logging[0].Level = "trace"
		// don't hit matrix.org when running tests!!!
		cfg.SigningKeyServer.KeyPerspectives = config.KeyPerspectives{}
	}

	j, err := yaml.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
