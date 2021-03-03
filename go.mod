module github.com/matrix-org/dendrite

require (
	github.com/DATA-DOG/go-sqlmock v1.5.0
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/Shopify/sarama v1.28.0
	github.com/gologme/log v1.2.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/lib/pq v1.9.0
	github.com/libp2p/go-libp2p v0.13.0
	github.com/libp2p/go-libp2p-circuit v0.4.0
	github.com/libp2p/go-libp2p-core v0.8.3
	github.com/libp2p/go-libp2p-gostream v0.3.1
	github.com/libp2p/go-libp2p-http v0.2.0
	github.com/libp2p/go-libp2p-kad-dht v0.11.1
	github.com/libp2p/go-libp2p-pubsub v0.4.1
	github.com/libp2p/go-libp2p-record v0.1.3
	github.com/lucas-clemente/quic-go v0.17.3
	github.com/matrix-org/dugong v0.0.0-20180820122854-51a565b5666b
	github.com/matrix-org/go-http-js-libp2p v0.0.0-20200518170932-783164aeeda4
	github.com/matrix-org/go-sqlite3-js v0.0.0-20200522092705-bc8506ccbcf3
	github.com/matrix-org/gomatrix v0.0.0-20200827122206-7dd5e2a05bcd
	github.com/matrix-org/gomatrixserverlib v0.0.0-20210302161955-6142fe3f8c2c
	github.com/matrix-org/naffka v0.0.0-20201009174903-d26a3b9cb161
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/mattn/go-sqlite3 v1.14.6
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/ngrok/sqlmw v0.0.0-20200129213757-d5c93a81bec6
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pressly/goose v2.7.0+incompatible
	github.com/prometheus/client_golang v1.9.0
	github.com/sirupsen/logrus v1.8.0
	github.com/tidwall/gjson v1.6.8
	github.com/tidwall/sjson v1.1.5
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible
	github.com/yggdrasil-network/yggdrasil-go v0.3.15-0.20210218094457-e77ca8019daa
	go.uber.org/atomic v1.7.0
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	gopkg.in/h2non/bimg.v1 v1.1.5
	gopkg.in/yaml.v2 v2.4.0
)

go 1.13
