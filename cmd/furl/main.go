package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"

	"github.com/matrix-org/gomatrixserverlib"
)

var requestFrom = flag.String("from", "", "the server name that the request should originate from")
var requestKey = flag.String("key", "matrix_key.pem", "the private key to use when signing the request")

func main() {
	flag.Parse()

	if requestFrom == nil || *requestFrom == "" {
		fmt.Println("expecting: furl -from ... [-key matrix_key.pem] https://path/to/url")
		fmt.Println("supported flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := ioutil.ReadFile(*requestKey)
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

	client := gomatrixserverlib.NewFederationClient(
		gomatrixserverlib.ServerName(*requestFrom),
		gomatrixserverlib.KeyID(keyBlock.Headers["Key-ID"]),
		privateKey,
		false,
	)

	u, err := url.Parse(flag.Arg(0))
	if err != nil {
		panic(err)
	}

	req := gomatrixserverlib.NewFederationRequest(
		"GET",
		gomatrixserverlib.ServerName(u.Host),
		u.RequestURI(),
	)

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
