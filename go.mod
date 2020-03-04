module github.com/matrix-org/dendrite

replace github.com/lib/pq => github.com/matrix-org/pq v1.3.2

replace github.com/prometheus/client_golang => ./prometheus

require (
	github.com/gorilla/mux v1.7.3
	github.com/lib/pq v1.2.0
	github.com/matrix-org/dugong v0.0.0-20171220115018-ea0a4690a0d5
	github.com/matrix-org/go-http-js-libp2p v0.0.0-20200304160008-4ec1129a00c4
	github.com/matrix-org/go-sqlite3-js v0.0.0-20200226144546-ea6ed5b90074
	github.com/matrix-org/gomatrix v0.0.0-20190528120928-7df988a63f26
	github.com/matrix-org/gomatrixserverlib v0.0.0-20200304110715-894c3c86ce3e
	github.com/matrix-org/naffka v0.0.0-20200127221512-0716baaabaf1
	github.com/matrix-org/util v0.0.0-20190711121626-527ce5ddefc7
	github.com/mattn/go-sqlite3 v2.0.2+incompatible
	github.com/nfnt/resize v0.0.0-20160724205520-891127d8d1b5
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.4.1
	github.com/sirupsen/logrus v1.4.2
	github.com/tidwall/gjson v1.6.0
	github.com/tidwall/pretty v1.0.1 // indirect
	github.com/uber/jaeger-client-go v2.22.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	gopkg.in/Shopify/sarama.v1 v1.20.1
	gopkg.in/h2non/bimg.v1 v1.0.18
	gopkg.in/yaml.v2 v2.2.5
)

go 1.13
