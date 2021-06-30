package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/codeclysm/extract"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var (
	flagTempDir = flag.String("tmp", "tmp", "Path to temporary directory to dump tarballs to")
)

func ptr(in string) *string {
	return &in
}

// downloadArchive downloads an arbitrary github archive of the form:
//   https://github.com/matrix-org/dendrite/archive/v0.3.11.tar.gz
// and re-tarballs it without the top-level directory which contains branch information. It inserts
// the contents of `dockerfile` as a root file `Dockerfile` in the re-tarballed directory such that
// you can directly feed the retarballed archive to `ImageBuild` to have it run said dockerfile.
// Returns the tarball buffer on success.
func downloadArchive(cli *http.Client, archiveURL string, dockerfile []byte) (*bytes.Buffer, error) {
	tmpDir := *flagTempDir
	resp, err := cli.Get(archiveURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got HTTP %d", resp.StatusCode)
	}
	os.Mkdir(tmpDir, os.ModePerm)
	defer os.RemoveAll(tmpDir)
	// dump the tarball temporarily, stripping the top-level directory
	err = extract.Archive(context.Background(), resp.Body, tmpDir, func(inPath string) string {
		// remove top level
		segments := strings.Split(inPath, "/")
		return strings.Join(segments[1:], "/")
	})
	if err != nil {
		return nil, err
	}
	// add top level Dockerfile
	err = ioutil.WriteFile(path.Join(tmpDir, "Dockerfile"), dockerfile, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to inject /Dockerfile: %w", err)
	}
	// now re-tarball it :/
	var tarball bytes.Buffer
	err = compress(*flagTempDir, &tarball)
	if err != nil {
		return nil, err
	}
	return &tarball, nil
}

func main() {
	flag.Parse()
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}
	// pull an archive, this contains a top-level directory which screws with the build context
	// which we need to fix up post download
	u := "https://github.com/matrix-org/dendrite/archive/v0.3.11.tar.gz"
	tarball, err := downloadArchive(httpClient, u, []byte("FROM golang:1.15\nRUN echo 'hello world'"))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s => %d bytes\n", u, tarball.Len())

	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	fmt.Println("building...")
	res, err := cli.ImageBuild(context.Background(), tarball, types.ImageBuildOptions{
		Tags: []string{"dendrite-upgrade"},
	})
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(body))
}
