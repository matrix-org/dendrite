package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"
)

func runTests(baseURL, branchName string) error {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}
	err := assertSuccess(httpClient.Post(
		fmt.Sprintf("%s/_matrix/client/r0/register", baseURL),
		"application/json",
		bytes.NewBufferString(fmt.Sprintf(`{"username":"%s","password":"%s","auth":{"type":"m.login.dummy"}}`, "alice", "this_is_a_long_password")),
	))
	if err != nil {
		return fmt.Errorf("failed to /register: %s", err)
	}
	return nil
}

func assertSuccess(res *http.Response, err error) error {
	if err != nil {
		return fmt.Errorf("response returned error: %s", err)
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("response returned HTTP %d - %s", res.StatusCode, dumpResponse(res))
	}
	return nil
}

func dumpResponse(res *http.Response) string {
	d, _ := httputil.DumpResponse(res, true)
	return string(d)
}
