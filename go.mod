module github.com/matrix-org/dendrite

replace github.com/nats-io/nats-server/v2 => github.com/neilalexander/nats-server/v2 v2.8.1-0.20220419100629-2278c94774f9

replace github.com/nats-io/nats.go => github.com/neilalexander/nats.go v1.13.1-0.20220419101051-b262d9f0be1e

require (
	github.com/Arceliar/ironwood v0.0.0-20211125050254-8951369625d0
	github.com/Arceliar/phony v0.0.0-20210209235338-dde1a8dca979
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/HdrHistogram/hdrhistogram-go v1.1.2 // indirect
	github.com/MFAshby/stdemuxerhook v1.0.0
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/codeclysm/extract v2.2.0+incompatible
	github.com/containerd/containerd v1.6.2 // indirect
	github.com/docker/docker v20.10.14+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/frankban/quicktest v1.14.3 // indirect
	github.com/getsentry/sentry-go v0.13.0
	github.com/gologme/log v1.3.0
	github.com/google/go-cmp v0.5.7
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.5.0
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/juju/testing v0.0.0-20220203020004-a0ff61f03494 // indirect
	github.com/lib/pq v1.10.5
	github.com/matrix-org/dugong v0.0.0-20210921133753-66e6b1c67e2e
	github.com/matrix-org/go-sqlite3-js v0.0.0-20220419092513-28aa791a1c91
	github.com/matrix-org/gomatrix v0.0.0-20210324163249-be2af5ef2e16
	github.com/matrix-org/gomatrixserverlib v0.0.0-20220408160933-cf558306b56f
	github.com/matrix-org/pinecone v0.0.0-20220408153826-2999ea29ed48
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/mattn/go-sqlite3 v1.14.10
	github.com/miekg/dns v1.1.31 // indirect
	github.com/nats-io/nats-server/v2 v2.7.4-0.20220309205833-773636c1c5bb
	github.com/nats-io/nats.go v1.14.0
	github.com/neilalexander/utp v0.1.1-0.20210727203401-54ae7b1cd5f9
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/ngrok/sqlmw v0.0.0-20211220175533-9d16fdc47b31
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opentracing/opentracing-go v1.2.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/pressly/goose v2.7.0+incompatible
	github.com/prometheus/client_golang v1.12.1
	github.com/sirupsen/logrus v1.8.1
	github.com/tidwall/gjson v1.14.0
	github.com/tidwall/sjson v1.2.4
	github.com/uber/jaeger-client-go v2.30.0+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/yggdrasil-network/yggdrasil-go v0.4.3
	go.uber.org/atomic v1.9.0
	golang.org/x/crypto v0.0.0-20220331220935-ae2d96664a29
	golang.org/x/image v0.0.0-20220321031419-a8550c1d254a
	golang.org/x/mobile v0.0.0-20220407111146-e579adbbc4a2
	golang.org/x/net v0.0.0-20220407224826-aac1ed45d8e3
	golang.org/x/sys v0.0.0-20220406163625-3f8b81556e12 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	gopkg.in/h2non/bimg.v1 v1.1.9
	gopkg.in/yaml.v2 v2.4.0
	nhooyr.io/websocket v1.8.7
)

go 1.16
