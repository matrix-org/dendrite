package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/codeclysm/extract"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

var (
	flagTempDir          = flag.String("tmp", "tmp", "Path to temporary directory to dump tarballs to")
	flagFrom             = flag.String("from", "HEAD-1", "The version to start from e.g '0.3.1'. If 'HEAD-N' then starts N versions behind HEAD.")
	flagTo               = flag.String("to", "HEAD", "The version to end on e.g '0.3.3'.")
	flagBuildConcurrency = flag.Int("build-concurrency", runtime.NumCPU(), "The amount of build concurrency when building images")
	flagHead             = flag.String("head", "", "Location to a dendrite repository to treat as HEAD instead of Github")
	flagDockerHost       = flag.String("docker-host", "localhost", "The hostname of the docker client. 'localhost' if running locally, 'host.docker.internal' if running in Docker.")
	alphaNumerics        = regexp.MustCompile("[^a-zA-Z0-9]+")
)

const HEAD = "HEAD"

// Embed the Dockerfile to use when building dendrite versions.
// We cannot use the dockerfile associated with the repo with each version sadly due to changes in
// Docker versions. Specifically, earlier Dendrite versions are incompatible with newer Docker clients
// due to the error:
//   When using COPY with more than one source file, the destination must be a directory and end with a /
// We need to run a postgres anyway, so use the dockerfile associated with Complement instead.
const Dockerfile = `FROM golang:1.16-stretch as build
RUN apt-get update && apt-get install -y postgresql
WORKDIR /build

# Copy the build context to the repo as this is the right dendrite code. This is different to the
# Complement Dockerfile which wgets a branch.
COPY . .

RUN go build ./cmd/dendrite-monolith-server
RUN go build ./cmd/generate-keys
RUN go build ./cmd/generate-config
RUN ./generate-config --ci > dendrite.yaml
RUN ./generate-keys --private-key matrix_key.pem --tls-cert server.crt --tls-key server.key

# Replace the connection string with a single postgres DB, using user/db = 'postgres' and no password
RUN sed -i "s%connection_string:.*$%connection_string: postgresql://postgres@localhost/postgres?sslmode=disable%g" dendrite.yaml 
# No password when connecting over localhost
RUN sed -i "s%127.0.0.1/32            md5%127.0.0.1/32            trust%g" /etc/postgresql/9.6/main/pg_hba.conf
# Bump up max conns for moar concurrency
RUN sed -i 's/max_connections = 100/max_connections = 2000/g' /etc/postgresql/9.6/main/postgresql.conf
RUN sed -i 's/max_open_conns:.*$/max_open_conns: 100/g' dendrite.yaml

# This entry script starts postgres, waits for it to be up then starts dendrite
RUN echo '\
#!/bin/bash -eu \n\
pg_lsclusters \n\
pg_ctlcluster 9.6 main start \n\
 \n\
until pg_isready \n\
do \n\
  echo "Waiting for postgres"; \n\
  sleep 1; \n\
done \n\
 \n\
sed -i "s/server_name: localhost/server_name: ${SERVER_NAME}/g" dendrite.yaml \n\
./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml \n\
' > run_dendrite.sh && chmod +x run_dendrite.sh

ENV SERVER_NAME=localhost
EXPOSE 8008 8448
CMD /build/run_dendrite.sh `

const dendriteUpgradeTestLabel = "dendrite_upgrade_test"

// downloadArchive downloads an arbitrary github archive of the form:
//   https://github.com/matrix-org/dendrite/archive/v0.3.11.tar.gz
// and re-tarballs it without the top-level directory which contains branch information. It inserts
// the contents of `dockerfile` as a root file `Dockerfile` in the re-tarballed directory such that
// you can directly feed the retarballed archive to `ImageBuild` to have it run said dockerfile.
// Returns the tarball buffer on success.
func downloadArchive(cli *http.Client, tmpDir, archiveURL string, dockerfile []byte) (*bytes.Buffer, error) {
	resp, err := cli.Get(archiveURL)
	if err != nil {
		return nil, err
	}
	// nolint:errcheck
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got HTTP %d", resp.StatusCode)
	}
	_ = os.RemoveAll(tmpDir)
	if err = os.Mkdir(tmpDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to make temporary directory: %s", err)
	}
	// nolint:errcheck
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
	err = compress(tmpDir, &tarball)
	if err != nil {
		return nil, err
	}
	return &tarball, nil
}

// buildDendrite builds Dendrite on the branchOrTagName given. Returns the image ID or an error
func buildDendrite(httpClient *http.Client, dockerClient *client.Client, tmpDir, branchOrTagName string) (string, error) {
	var tarball *bytes.Buffer
	var err error
	// If a custom HEAD location is given, use that, else pull from github. Mostly useful for CI
	// where we want to use the working directory.
	if branchOrTagName == HEAD && *flagHead != "" {
		log.Printf("%s: Using %s as HEAD", branchOrTagName, *flagHead)
		// add top level Dockerfile
		err = ioutil.WriteFile(path.Join(*flagHead, "Dockerfile"), []byte(Dockerfile), os.ModePerm)
		if err != nil {
			return "", fmt.Errorf("custom HEAD: failed to inject /Dockerfile: %w", err)
		}
		// now tarball it
		var buffer bytes.Buffer
		err = compress(*flagHead, &buffer)
		if err != nil {
			return "", fmt.Errorf("failed to tarball custom HEAD %s : %s", *flagHead, err)
		}
		tarball = &buffer
	} else {
		log.Printf("%s: Downloading version %s to %s\n", branchOrTagName, branchOrTagName, tmpDir)
		// pull an archive, this contains a top-level directory which screws with the build context
		// which we need to fix up post download
		u := fmt.Sprintf("https://github.com/matrix-org/dendrite/archive/%s.tar.gz", branchOrTagName)
		tarball, err = downloadArchive(httpClient, tmpDir, u, []byte(Dockerfile))
		if err != nil {
			return "", fmt.Errorf("failed to download archive %s: %w", u, err)
		}
		log.Printf("%s: %s => %d bytes\n", branchOrTagName, u, tarball.Len())
	}

	log.Printf("%s: Building version %s\n", branchOrTagName, branchOrTagName)
	res, err := dockerClient.ImageBuild(context.Background(), tarball, types.ImageBuildOptions{
		Tags: []string{"dendrite-upgrade"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to start building image: %s", err)
	}
	// nolint:errcheck
	defer res.Body.Close()
	decoder := json.NewDecoder(res.Body)
	// {"aux":{"ID":"sha256:247082c717963bc2639fc2daed08838d67811ea12356cd4fda43e1ffef94f2eb"}}
	var imageID string
	for decoder.More() {
		var dl struct {
			Stream string                 `json:"stream"`
			Aux    map[string]interface{} `json:"aux"`
		}
		if err := decoder.Decode(&dl); err != nil {
			return "", fmt.Errorf("failed to decode build image output line: %w", err)
		}
		log.Printf("%s: %s", branchOrTagName, dl.Stream)
		if dl.Aux != nil {
			imgID, ok := dl.Aux["ID"]
			if ok {
				imageID = imgID.(string)
			}
		}
	}
	return imageID, nil
}

func getAndSortVersionsFromGithub(httpClient *http.Client) (semVers []*semver.Version, err error) {
	u := "https://api.github.com/repos/matrix-org/dendrite/tags"
	res, err := httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s returned HTTP %d", u, res.StatusCode)
	}
	resp := []struct {
		Name string `json:"name"`
	}{}
	if err = json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, err
	}
	for _, r := range resp {
		v, err := semver.NewVersion(r.Name)
		if err != nil {
			continue // not a semver, that's ok and isn't an error, we allow tags that aren't semvers
		}
		semVers = append(semVers, v)
	}
	sort.Sort(semver.Collection(semVers))
	return semVers, nil
}

func calculateVersions(cli *http.Client, from, to string) []string {
	semvers, err := getAndSortVersionsFromGithub(cli)
	if err != nil {
		log.Fatalf("failed to collect semvers from github: %s", err)
	}
	// snip the lower bound depending on --from
	if from != "" {
		if strings.HasPrefix(from, "HEAD-") {
			var headN int
			headN, err = strconv.Atoi(strings.TrimPrefix(from, "HEAD-"))
			if err != nil {
				log.Fatalf("invalid --from, try 'HEAD-1'")
			}
			if headN >= len(semvers) {
				log.Fatalf("only have %d versions, but asked to go to HEAD-%d", len(semvers), headN)
			}
			if headN > 0 {
				semvers = semvers[len(semvers)-headN:]
			}
		} else {
			fromVer, err := semver.NewVersion(from)
			if err != nil {
				log.Fatalf("invalid --from: %s", err)
			}
			i := 0
			for i = 0; i < len(semvers); i++ {
				if semvers[i].LessThan(fromVer) {
					continue
				}
				break
			}
			semvers = semvers[i:]
		}
	}
	if to != "" && to != HEAD {
		toVer, err := semver.NewVersion(to)
		if err != nil {
			log.Fatalf("invalid --to: %s", err)
		}
		var i int
		for i = len(semvers) - 1; i >= 0; i-- {
			if semvers[i].GreaterThan(toVer) {
				continue
			}
			break
		}
		semvers = semvers[:i+1]
	}
	var versions []string
	for _, sv := range semvers {
		versions = append(versions, sv.Original())
	}
	if to == HEAD {
		versions = append(versions, HEAD)
	}
	return versions
}

func buildDendriteImages(httpClient *http.Client, dockerClient *client.Client, baseTempDir string, concurrency int, branchOrTagNames []string) map[string]string {
	// concurrently build all versions, this can be done in any order. The mutex protects the map
	branchToImageID := make(map[string]string)
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(concurrency)
	ch := make(chan string, len(branchOrTagNames))
	for _, branchName := range branchOrTagNames {
		ch <- branchName
	}
	close(ch)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for branchName := range ch {
				tmpDir := baseTempDir + alphaNumerics.ReplaceAllString(branchName, "")
				imgID, err := buildDendrite(httpClient, dockerClient, tmpDir, branchName)
				if err != nil {
					log.Fatalf("%s: failed to build dendrite image: %s", branchName, err)
				}
				mu.Lock()
				branchToImageID[branchName] = imgID
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return branchToImageID
}

func runImage(dockerClient *client.Client, volumeName, version, imageID string) (csAPIURL, containerID string, err error) {
	log.Printf("%s: running image %s\n", version, imageID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	body, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: imageID,
		Env:   []string{"SERVER_NAME=hs1"},
		Labels: map[string]string{
			dendriteUpgradeTestLabel: "yes",
		},
	}, &container.HostConfig{
		PublishAllPorts: true,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/var/lib/postgresql/9.6/main",
			},
		},
	}, nil, nil, "dendrite_upgrade_test_"+version)
	if err != nil {
		return "", "", fmt.Errorf("failed to ContainerCreate: %s", err)
	}
	containerID = body.ID

	err = dockerClient.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to ContainerStart: %s", err)
	}
	inspect, err := dockerClient.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", "", err
	}
	csapiPortInfo, ok := inspect.NetworkSettings.Ports[nat.Port("8008/tcp")]
	if !ok {
		return "", "", fmt.Errorf("port 8008 not exposed - exposed ports: %v", inspect.NetworkSettings.Ports)
	}
	baseURL := fmt.Sprintf("http://%s:%s", *flagDockerHost, csapiPortInfo[0].HostPort)
	versionsURL := fmt.Sprintf("%s/_matrix/client/versions", baseURL)
	// hit /versions to check it is up
	var lastErr error
	for i := 0; i < 500; i++ {
		res, err := http.Get(versionsURL)
		if err != nil {
			lastErr = fmt.Errorf("GET %s => error: %s", versionsURL, err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if res.StatusCode != 200 {
			lastErr = fmt.Errorf("GET %s => HTTP %s", versionsURL, res.Status)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		logs, err := dockerClient.ContainerLogs(context.Background(), containerID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
		})
		// ignore errors when cannot get logs, it's just for debugging anyways
		if err == nil {
			logbody, err := ioutil.ReadAll(logs)
			if err == nil {
				log.Printf("Container logs:\n\n%s\n\n", string(logbody))
			}
		}
	}
	return baseURL, containerID, lastErr
}

func destroyContainer(dockerClient *client.Client, containerID string) {
	err := dockerClient.ContainerRemove(context.TODO(), containerID, types.ContainerRemoveOptions{
		Force: true,
	})
	if err != nil {
		log.Printf("failed to remove container %s : %s", containerID, err)
	}
}

func loadAndRunTests(dockerClient *client.Client, volumeName, v string, branchToImageID map[string]string) error {
	csAPIURL, containerID, err := runImage(dockerClient, volumeName, v, branchToImageID[v])
	if err != nil {
		return fmt.Errorf("failed to run container for branch %v: %v", v, err)
	}
	defer destroyContainer(dockerClient, containerID)
	log.Printf("URL %s -> %s \n", csAPIURL, containerID)
	if err = runTests(csAPIURL, v); err != nil {
		return fmt.Errorf("failed to run tests on version %s: %s", v, err)
	}
	return nil
}

func verifyTests(dockerClient *client.Client, volumeName string, versions []string, branchToImageID map[string]string) error {
	lastVer := versions[len(versions)-1]
	csAPIURL, containerID, err := runImage(dockerClient, volumeName, lastVer, branchToImageID[lastVer])
	if err != nil {
		return fmt.Errorf("failed to run container for branch %v: %v", lastVer, err)
	}
	defer destroyContainer(dockerClient, containerID)
	return verifyTestsRan(csAPIURL, versions)
}

// cleanup old containers/volumes from a previous run
func cleanup(dockerClient *client.Client) {
	// ignore all errors, we are just cleaning up and don't want to fail just because we fail to cleanup
	containers, _ := dockerClient.ContainerList(context.Background(), types.ContainerListOptions{
		Filters: label(dendriteUpgradeTestLabel),
	})
	for _, c := range containers {
		s := time.Second
		_ = dockerClient.ContainerStop(context.Background(), c.ID, &s)
		_ = dockerClient.ContainerRemove(context.Background(), c.ID, types.ContainerRemoveOptions{
			Force: true,
		})
	}
	_ = dockerClient.VolumeRemove(context.Background(), "dendrite_upgrade_test", true)
}

func label(in string) filters.Args {
	f := filters.NewArgs()
	f.Add("label", in)
	return f
}

func main() {
	flag.Parse()
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Fatalf("failed to make docker client: %s", err)
	}
	if *flagFrom == "" {
		flag.Usage()
		os.Exit(1)
	}
	cleanup(dockerClient)
	versions := calculateVersions(httpClient, *flagFrom, *flagTo)
	log.Printf("Testing dendrite versions: %v\n", versions)

	branchToImageID := buildDendriteImages(httpClient, dockerClient, *flagTempDir, *flagBuildConcurrency, versions)

	// make a shared postgres volume
	volume, err := dockerClient.VolumeCreate(context.Background(), volume.VolumeCreateBody{
		Name: "dendrite_upgrade_test",
		Labels: map[string]string{
			dendriteUpgradeTestLabel: "yes",
		},
	})
	if err != nil {
		log.Fatalf("failed to make docker volume: %s", err)
	}

	failed := false
	defer func() {
		perr := recover()
		log.Println("removing postgres volume")
		verr := dockerClient.VolumeRemove(context.Background(), volume.Name, true)
		if perr == nil {
			perr = verr
		}
		if perr != nil {
			panic(perr)
		}
		if failed {
			os.Exit(1)
		}
	}()

	// run through images sequentially
	for _, v := range versions {
		if err = loadAndRunTests(dockerClient, volume.Name, v, branchToImageID); err != nil {
			log.Printf("failed to run tests for %v: %s\n", v, err)
			failed = true
			break
		}
	}
	if err := verifyTests(dockerClient, volume.Name, versions, branchToImageID); err != nil {
		log.Printf("failed to verify test results: %s", err)
		failed = true
	}
}
