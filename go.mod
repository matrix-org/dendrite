module github.com/matrix-org/dendrite

replace github.com/nats-io/nats-server/v2 => github.com/neilalexander/nats-server/v2 v2.8.3-0.20220513095553-73a9a246d34f

replace github.com/nats-io/nats.go => github.com/neilalexander/nats.go v1.13.1-0.20220621084451-ac518c356673

require (
	github.com/Arceliar/ironwood v0.0.0-20220306165321-319147a02d98
	github.com/Arceliar/phony v0.0.0-20210209235338-dde1a8dca979
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/MFAshby/stdemuxerhook v1.0.0
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/codeclysm/extract v2.2.0+incompatible
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v20.10.16+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/docker/go-units v0.4.0 // indirect
	github.com/frankban/quicktest v1.14.3 // indirect
	github.com/getsentry/sentry-go v0.13.0
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/gologme/log v1.3.0
	github.com/google/go-cmp v0.5.8
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/juju/testing v0.0.0-20220203020004-a0ff61f03494 // indirect
	github.com/kardianos/minwinsvc v1.0.0
	github.com/lib/pq v1.10.5
	github.com/matrix-org/dugong v0.0.0-20210921133753-66e6b1c67e2e
	github.com/matrix-org/go-sqlite3-js v0.0.0-20220419092513-28aa791a1c91
	github.com/matrix-org/gomatrix v0.0.0-20210324163249-be2af5ef2e16
	github.com/matrix-org/gomatrixserverlib v0.0.0-20220613132209-aedb3fbb511a
	github.com/matrix-org/pinecone v0.0.0-20220408153826-2999ea29ed48
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/mattn/go-sqlite3 v1.14.13
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/miekg/dns v1.1.49 // indirect
	github.com/moby/term v0.0.0-20210610120745-9d4ed1856297 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/nats-io/nats-server/v2 v2.7.4-0.20220309205833-773636c1c5bb
	github.com/nats-io/nats.go v1.14.0
	github.com/neilalexander/utp v0.1.1-0.20210727203401-54ae7b1cd5f9
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/ngrok/sqlmw v0.0.0-20220520173518-97c9c04efc79
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799 // indirect
	github.com/opentracing/opentracing-go v1.2.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/pressly/goose v2.7.0+incompatible
	github.com/prometheus/client_golang v1.12.2
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.7.1
	github.com/tidwall/gjson v1.14.1
	github.com/tidwall/sjson v1.2.4
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/yggdrasil-network/yggdrasil-go v0.4.3
	go.uber.org/atomic v1.9.0
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/image v0.0.0-20220413100746-70e8d0d3baa9
	golang.org/x/mobile v0.0.0-20220518205345-8578da9835fd
	golang.org/x/net v0.0.0-20220524220425-1d687d428aca
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	golang.org/x/term v0.0.0-20220526004731-065cf7ba2467
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/h2non/bimg.v1 v1.1.9
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0 // indirect
	gotest.tools/v3 v3.0.3 // indirect
	nhooyr.io/websocket v1.8.7
)

go 1.16
