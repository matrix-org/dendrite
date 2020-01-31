// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package test

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

// Request contains the information necessary to issue a request and test its result
type Request struct {
	Req              *http.Request
	WantedBody       string
	WantedStatusCode int
	LastErr          *LastRequestErr
}

// LastRequestErr is a synchronised error wrapper
// Useful for obtaining the last error from a set of requests
type LastRequestErr struct {
	sync.Mutex
	Err error
}

// Set sets the error
func (r *LastRequestErr) Set(err error) {
	r.Lock()
	defer r.Unlock()
	r.Err = err
}

// Get gets the error
func (r *LastRequestErr) Get() error {
	r.Lock()
	defer r.Unlock()
	return r.Err
}

// CanonicalJSONInput canonicalises a slice of JSON strings
// Useful for test input
func CanonicalJSONInput(jsonData []string) []string {
	for i := range jsonData {
		jsonBytes, err := gomatrixserverlib.CanonicalJSON([]byte(jsonData[i]))
		if err != nil && err != io.EOF {
			panic(err)
		}
		jsonData[i] = string(jsonBytes)
	}
	return jsonData
}

// Do issues a request and checks the status code and body of the response
func (r *Request) Do() (err error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	res, err := client.Do(r.Req)

	defer (func() {
		finalErr := res.Body.Close()
		if err != nil && finalErr != nil {
			err = fmt.Errorf("%s\n%s", err, finalErr)
		} else if err == nil {
			err = finalErr
		}
	})()

	if err != nil {
		return err
	}

	if res.StatusCode != r.WantedStatusCode {
		return fmt.Errorf("incorrect status code. Expected: %d  Got: %d", r.WantedStatusCode, res.StatusCode)
	}

	if r.WantedBody != "" {
		resBytes, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}
		jsonBytes, err := gomatrixserverlib.CanonicalJSON(resBytes)
		if err != nil {
			return err
		}
		if string(jsonBytes) != r.WantedBody {
			return fmt.Errorf("returned wrong bytes. Expected:\n%s\n\nGot:\n%s", r.WantedBody, string(jsonBytes))
		}
	}

	return nil
}

// DoUntilSuccess blocks and repeats the same request until the response returns the desired status code and body.
// It then closes the given channel and returns.
func (r *Request) DoUntilSuccess(done chan error) {
	r.LastErr = &LastRequestErr{}
	for {
		if err := r.Do(); err != nil {
			r.LastErr.Set(err)
			time.Sleep(1 * time.Second) // don't tightloop
			continue
		}
		close(done)
		return
	}
}

// Run repeatedly issues a request until success, error or a timeout is reached
func (r *Request) Run(label string, timeout time.Duration, serverCmdChan chan error) {
	fmt.Printf("==TESTING== %v (timeout: %v)\n", label, timeout)
	done := make(chan error, 1)

	// We need to wait for the server to:
	// - have connected to the database
	// - have created the tables
	// - be listening on the given port
	go r.DoUntilSuccess(done)

	// wait for one of:
	// - the test to pass (done channel is closed)
	// - the server to exit with an error (error sent on serverCmdChan)
	// - our test timeout to expire
	// We don't need to clean up since the main() function handles that in the event we panic
	select {
	case <-time.After(timeout):
		fmt.Printf("==TESTING== %v TIMEOUT\n", label)
		if reqErr := r.LastErr.Get(); reqErr != nil {
			fmt.Println("Last /sync request error:")
			fmt.Println(reqErr)
		}
		panic(fmt.Sprintf("%v server timed out", label))
	case err := <-serverCmdChan:
		if err != nil {
			fmt.Println("=============================================================================================")
			fmt.Printf("%v server failed to run. If failing with 'pq: password authentication failed for user' try:", label)
			fmt.Println("    export PGHOST=/var/run/postgresql")
			fmt.Println("=============================================================================================")
			panic(err)
		}
	case <-done:
		fmt.Printf("==TESTING== %v PASSED\n", label)
	}
}
