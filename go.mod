module github.com/matrix-org/dendrite

require (
	github.com/Arceliar/ironwood v0.0.0-20210619124114-6ad55cae5031
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Shopify/sarama v1.29.1
	github.com/codeclysm/extract v2.2.0+incompatible
	github.com/containerd/containerd v1.5.5 // indirect
	github.com/docker/docker v20.10.7+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/getsentry/sentry-go v0.11.0
	github.com/gologme/log v1.2.0
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/h2non/filetype v1.1.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4
	github.com/juju/testing v0.0.0-20210324180055-18c50b0c2098 // indirect
	github.com/lib/pq v1.10.1
	github.com/libp2p/go-libp2p v0.13.0
	github.com/libp2p/go-libp2p-circuit v0.4.0
	github.com/libp2p/go-libp2p-core v0.8.3
	github.com/libp2p/go-libp2p-gostream v0.3.1
	github.com/libp2p/go-libp2p-http v0.2.0
	github.com/libp2p/go-libp2p-kad-dht v0.11.1
	github.com/libp2p/go-libp2p-pubsub v0.4.1
	github.com/libp2p/go-libp2p-record v0.1.3
	github.com/lucas-clemente/quic-go v0.22.0
	github.com/matrix-org/dugong v0.0.0-20210603171012-8379174dca81
	github.com/matrix-org/go-http-js-libp2p v0.0.0-20200518170932-783164aeeda4
	github.com/matrix-org/go-sqlite3-js v0.0.0-20210709140738-b0d1ba599a6d
	github.com/matrix-org/gomatrix v0.0.0-20210324163249-be2af5ef2e16
	github.com/matrix-org/gomatrixserverlib v0.0.0-20210817115641-f9416ac1a723
	github.com/matrix-org/naffka v0.0.0-20210623111924-14ff508b58e0
	github.com/matrix-org/pinecone v0.0.0-20210910134625-4ec11c22f2c8
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/matryer/is v1.4.0
	github.com/mattn/go-sqlite3 v1.14.8
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/neilalexander/utp v0.1.1-0.20210727203401-54ae7b1cd5f9
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/ngrok/sqlmw v0.0.0-20200129213757-d5c93a81bec6
	github.com/opentracing/opentracing-go v1.2.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/errors v0.9.1
	github.com/pressly/goose v2.7.0+incompatible
	github.com/prometheus/client_golang v1.11.0
	github.com/sirupsen/logrus v1.8.1
	github.com/tidwall/gjson v1.8.1
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tidwall/sjson v1.1.7
	github.com/uber/jaeger-client-go v2.29.1+incompatible
	github.com/uber/jaeger-lib v2.4.1+incompatible
	github.com/yggdrasil-network/yggdrasil-go v0.4.1-0.20210715083903-52309d094c00
	go.uber.org/atomic v1.9.0
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97
	golang.org/x/mobile v0.0.0-20210716004757-34ab1303b554
	golang.org/x/net v0.0.0-20210726213435-c6fcb2dbf985
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b
	gopkg.in/h2non/bimg.v1 v1.1.5
	gopkg.in/yaml.v2 v2.4.0
	nhooyr.io/websocket v1.8.7
)

go 1.15
