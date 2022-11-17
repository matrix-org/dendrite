package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/matrix-org/gomatrixserverlib"
)

var requestFrom = flag.String("from", "", "the server name that the request should originate from")
var requestKey = flag.String("key", "matrix_key.pem", "the private key to use when signing the request")
var requestPost = flag.Bool("post", false, "send a POST request instead of GET (pipe input into stdin or type followed by Ctrl-D)")

func main() {
	flag.Parse()

	if requestFrom == nil || *requestFrom == "" {
		fmt.Println("expecting: furl -from origin.com [-key matrix_key.pem] https://path/to/url")
		fmt.Println("supported flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := os.ReadFile(*requestKey)
	if err != nil {
		panic(err)
	}

	var privateKey ed25519.PrivateKey
	keyBlock, _ := pem.Decode(data)
	if keyBlock == nil {
		panic("keyBlock is nil")
	}
	if keyBlock.Type == "MATRIX PRIVATE KEY" {
		_, privateKey, err = ed25519.GenerateKey(bytes.NewReader(keyBlock.Bytes))
		if err != nil {
			panic(err)
		}
	} else {
		panic("unexpected key block")
	}

	serverName := gomatrixserverlib.ServerName(*requestFrom)
	client := gomatrixserverlib.NewFederationClient(
		[]*gomatrixserverlib.SigningIdentity{
			{
				ServerName: serverName,
				KeyID:      gomatrixserverlib.KeyID(keyBlock.Headers["Key-ID"]),
				PrivateKey: privateKey,
			},
		},
	)

	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		panic(err)
	}

	var bodyObj interface{}
	var bodyBytes []byte
	method := "GET"
	if *requestPost {
		method = "POST"
		fmt.Println("Waiting for JSON input. Press Enter followed by Ctrl-D when done...")

		scan := bufio.NewScanner(os.Stdin)
		for scan.Scan() {
			bytes := scan.Bytes()
			bodyBytes = append(bodyBytes, bytes...)
		}
		fmt.Println("Done!")
		if err = json.Unmarshal(bodyBytes, &bodyObj); err != nil {
			panic(err)
		}
	}

	req := gomatrixserverlib.NewFederationRequest(
		method,
		serverName,
		gomatrixserverlib.ServerName(u.Host),
		u.RequestURI(),
	)

	if *requestPost {
		if err = req.SetContent(bodyObj); err != nil {
			panic(err)
		}
	}

	if err = req.Sign(
		gomatrixserverlib.ServerName(*requestFrom),
		gomatrixserverlib.KeyID(keyBlock.Headers["Key-ID"]),
		privateKey,
	); err != nil {
		panic(err)
	}

	httpReq, err := req.HTTPRequest()
	if err != nil {
		panic(err)
	}

	var res interface{}
	err = client.DoRequestAndParseResponse(
		context.TODO(),
		httpReq,
		&res,
	)
	if err != nil {
		panic(err)
	}

	j, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}
