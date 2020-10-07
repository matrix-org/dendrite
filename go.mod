module github.com/matrix-org/dendrite

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/Shopify/sarama v1.27.0
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/gologme/log v1.2.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/lib/pq v1.8.0
	github.com/libp2p/go-libp2p v0.11.0
	github.com/libp2p/go-libp2p-circuit v0.3.1
	github.com/libp2p/go-libp2p-core v0.6.1
	github.com/libp2p/go-libp2p-gostream v0.2.1
	github.com/libp2p/go-libp2p-http v0.1.5
	github.com/libp2p/go-libp2p-kad-dht v0.9.0
	github.com/libp2p/go-libp2p-pubsub v0.3.5
	github.com/libp2p/go-libp2p-record v0.1.3
	github.com/libp2p/go-yamux v1.3.9 // indirect
	github.com/lucas-clemente/quic-go v0.17.3
	github.com/matrix-org/dugong v0.0.0-20180820122854-51a565b5666b
	github.com/matrix-org/go-http-js-libp2p v0.0.0-20200518170932-783164aeeda4
	github.com/matrix-org/go-sqlite3-js v0.0.0-20200522092705-bc8506ccbcf3
	github.com/matrix-org/gomatrix v0.0.0-20200827122206-7dd5e2a05bcd
	github.com/matrix-org/gomatrixserverlib v0.0.0-20201006143701-222e7423a5e3
	github.com/matrix-org/naffka v0.0.0-20200901083833-bcdd62999a91
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/mattn/go-sqlite3 v1.14.2
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/ngrok/sqlmw v0.0.0-20200129213757-d5c93a81bec6
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pressly/goose v2.7.0-rc5+incompatible
	github.com/prometheus/client_golang v1.7.1
	github.com/sirupsen/logrus v1.6.0
	github.com/tidwall/gjson v1.6.1
	github.com/tidwall/sjson v1.1.1
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	github.com/yggdrasil-network/yggdrasil-go v0.3.15-0.20201006093556-760d9a7fd5ee
	go.uber.org/atomic v1.6.0
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a
	gopkg.in/h2non/bimg.v1 v1.1.4
	gopkg.in/yaml.v2 v2.3.0
)

go 1.13
