package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var (
	flagSkipVerify bool   //  -k, --insecure           Allow insecure server connections
	flagMethod     string //  -X, --request <method>   Specify request method to use
	flagData       string // -d, --data <data>        HTTP POST data

	flagMatrixKey string
	flagOrigin    string

	flagPort int
)

func init() {
	flag.BoolVar(&flagSkipVerify, "insecure", false, "Allow insecure server connections")
	flag.BoolVar(&flagSkipVerify, "k", false, "Allow insecure server connections")

	flag.StringVar(&flagMethod, "X", "GET", "Specify HTTP request method to use")
	flag.StringVar(&flagMethod, "request", "GET", "Specify HTTP request method to use")

	flag.StringVar(&flagData, "d", "", "HTTP JSON body data. If you start the data with the letter @, the rest should be a filename.")
	flag.StringVar(&flagData, "data", "", "HTTP JSON body data. If you start the data with the letter @, the rest should be a filename.")

	flag.StringVar(&flagMatrixKey, "M", "matrix_key.pem", "The private key to use when signing the request")
	flag.StringVar(&flagMatrixKey, "key", "matrix_key.pem", "The private key to use when signing the request")

	flag.StringVar(&flagOrigin, "O", "", "The server name that the request should originate from. The remote server will use this to request server keys. There MUST be a TLS listener at the .well-known address for this server name, i.e it needs to be pointing to a real homeserver. If blank, furl will self-host this on a random high numbered port, but only if the target is localhost. Use $PORT in request URLs/bodies to substitute the port number in.")
	flag.StringVar(&flagOrigin, "origin", "", "The server name that the request should originate from. The remote server will use this to request server keys. There MUST be a TLS listener at the .well-known address for this server name, i.e it needs to be pointing to a real homeserver. If blank, furl will self-host this on a random high numbered port, but only if the target is localhost. Use $PORT in request URLs/bodies to substitute the port number in.")

	flag.IntVar(&flagPort, "p", 0, "Port to self-host on. If set, always self-hosts. Required because sometimes requests need the same origin.")
}

type args struct {
	SkipVerify   bool
	Method       string
	Data         []byte
	MatrixKey    ed25519.PrivateKey
	MatrixKeyID  gomatrixserverlib.KeyID
	Origin       spec.ServerName
	SelfHostKey  bool
	SelfHostPort int
	TargetURL    *url.URL
}

func processArgs() (*args, error) {
	if len(flag.Arg(0)) == 0 {
		return nil, fmt.Errorf("furl [-k] [-X GET|PUT|POST|DELETE] [-d @filename|{\"inline\":\"json\"}] [-M matrix_key.pem] [-O localhost] https://federation-server.url/_matrix/.../")
	}
	targetURL, err := url.Parse(flag.Arg(0))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %s", err)
	}

	// load .pem file
	data, err := os.ReadFile(flagMatrixKey)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode(data)
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid pem file: %s", flagMatrixKey)
	}
	if keyBlock.Type != "MATRIX PRIVATE KEY" {
		return nil, fmt.Errorf("pem file bad block type, want MATRIX PRIVATE KEY got %s", keyBlock.Type)
	}
	_, privateKey, err := ed25519.GenerateKey(bytes.NewReader(keyBlock.Bytes))
	if err != nil {
		return nil, err
	}

	var a args
	a.MatrixKey = privateKey
	a.MatrixKeyID = gomatrixserverlib.KeyID(keyBlock.Headers["Key-ID"])

	a.SkipVerify = flagSkipVerify
	a.Method = strings.ToUpper(flagMethod)
	a.Origin = spec.ServerName(flagOrigin)
	a.SelfHostPort = flagPort
	a.TargetURL = targetURL
	a.SelfHostKey = a.SelfHostPort != 0 || (a.Origin == "" && a.TargetURL.Hostname() == "localhost")

	// load data
	isFile := strings.HasPrefix(flagData, "@")
	if isFile {
		a.Data, err = os.ReadFile(flagData[1:])
		if err != nil {
			return nil, fmt.Errorf("failed to read file '%s': %s", flagData[1:], err)
		}
	} else if len(flagData) > 0 {
		a.Data = []byte(flagData)
	}

	return &a, nil
}

func main() {
	flag.Parse()
	a, err := processArgs()
	if err != nil {
		fmt.Println(err.Error())
		flag.PrintDefaults()
		os.Exit(1)
	}

	if a.SelfHostKey {
		fmt.Printf("Self-hosting key...")
		apiURL, cancel := test.ListenAndServe(tt{}, http.DefaultServeMux, true, a.SelfHostPort)
		defer cancel()
		parsedURL, _ := url.Parse(apiURL)
		a.Origin = spec.ServerName(parsedURL.Host)
		fmt.Printf(" OK on %s\n", a.Origin)

		// handle the request when it comes in
		pubKey := a.MatrixKey.Public().(ed25519.PublicKey)
		serverKey := gomatrixserverlib.ServerKeyFields{
			ServerName:   a.Origin,
			ValidUntilTS: spec.AsTimestamp(time.Now().Add(2 * time.Minute)),
			VerifyKeys: map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
				a.MatrixKeyID: {
					Key: spec.Base64Bytes(pubKey),
				},
			},
		}
		serverKeyBytes, err := json.Marshal(serverKey)
		if err != nil {
			panic(err)
		}
		signedBytes, err := gomatrixserverlib.SignJSON(string(a.Origin), a.MatrixKeyID, a.MatrixKey, serverKeyBytes)
		if err != nil {
			panic(err)
		}
		resp := map[string]interface{}{
			"server_keys": []json.RawMessage{signedBytes},
		}
		respBytes, err := json.Marshal(resp)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Will return %s\n", string(respBytes))
		http.HandleFunc("/_matrix/key/v2/query", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(respBytes)
		})

		// replace anything with $PORT
		port := parsedURL.Port()
		a.TargetURL, err = url.Parse(strings.ReplaceAll(a.TargetURL.String(), "$PORT", port))
		if err != nil {
			panic(err)
		}
		if a.Data != nil {
			data := string(a.Data)
			data = strings.ReplaceAll(data, "$PORT", port)
			a.Data = []byte(data)
		}
	}

	client := fclient.NewFederationClient(
		[]*fclient.SigningIdentity{
			{
				ServerName: a.Origin,
				KeyID:      a.MatrixKeyID,
				PrivateKey: a.MatrixKey,
			},
		}, fclient.WithSkipVerify(a.SkipVerify),
	)

	req := fclient.NewFederationRequest(
		a.Method,
		a.Origin,
		spec.ServerName(a.TargetURL.Host),
		a.TargetURL.RequestURI(),
	)
	if a.Data != nil {
		var jsonData interface{}
		if err := json.Unmarshal(a.Data, &jsonData); err != nil {
			fmt.Printf("Supplied data is not valid json: %s\n", err)
			os.Exit(1)
		}
		if err = req.SetContent(jsonData); err != nil {
			panic(err) // should be impossible as we just checked it was valid JSON
		}
	}
	if err = req.Sign(a.Origin, a.MatrixKeyID, a.MatrixKey); err != nil {
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
		mxerr, ok := err.(gomatrix.HTTPError)
		if ok {
			fmt.Printf("Server returned HTTP %d\n", mxerr.Code)
			fmt.Println(mxerr.Message)
			fmt.Println(mxerr.WrappedError)
		} else {
			panic(err)
		}
		os.Exit(1)
	}

	j, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		panic(err)
	}

	fmt.Println(string(j))
}

type tt struct{}

func (t tt) Logf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
func (t tt) Errorf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}
func (t tt) Fatalf(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
	os.Exit(2)
}
func (t tt) TempDir() string {
	return os.TempDir()
}
