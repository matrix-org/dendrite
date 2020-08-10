package main

import (
	"fmt"

	"github.com/hjson/hjson-go"
	"github.com/matrix-org/dendrite/internal/config"
)

func main() {
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Logging = []config.LogrusHook{
		{
			Type:  "file",
			Level: "info",
			Params: map[string]interface{}{
				"path": "/var/log/dendrite",
			},
		},
	}
	/*
			- server_name: matrix.org
		      keys:
		        - key_id: ed25519:auto
		          public_key: Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw
		        - key_id: ed25519:a_RXGa
		          public_key: l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ
	*/
	cfg.ServerKeyAPI.KeyPerspectives = append(cfg.ServerKeyAPI.KeyPerspectives, config.KeyPerspective{
		ServerName: "matrix.org",
		Keys: []config.KeyPerspectiveTrustKeys{
			{
				KeyID:     "ed25519:auto",
				PublicKey: "Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw",
			},
			{
				KeyID:     "ed25519:a_RXGa",
				PublicKey: "l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ",
			},
		},
	})

	j, err := hjson.Marshal(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
