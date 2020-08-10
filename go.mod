module github.com/matrix-org/dendrite

require (
	github.com/Shopify/sarama v1.26.1
	github.com/circonus-labs/circonus-gometrics v2.3.1+incompatible
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/gologme/log v1.2.0
	github.com/gorilla/mux v1.7.3
	github.com/hashicorp/golang-lru v0.5.4
	github.com/lib/pq v1.2.0
	github.com/libp2p/go-libp2p v0.6.0
	github.com/libp2p/go-libp2p-circuit v0.1.4
	github.com/libp2p/go-libp2p-core v0.5.0
	github.com/libp2p/go-libp2p-gostream v0.2.1
	github.com/libp2p/go-libp2p-http v0.1.5
	github.com/libp2p/go-libp2p-kad-dht v0.5.0
	github.com/libp2p/go-libp2p-pubsub v0.2.5
	github.com/libp2p/go-libp2p-record v0.1.2
	github.com/libp2p/go-yamux v1.3.7 // indirect
	github.com/lucas-clemente/quic-go v0.17.3
	github.com/matrix-org/dugong v0.0.0-20171220115018-ea0a4690a0d5
	github.com/matrix-org/go-http-js-libp2p v0.0.0-20200518170932-783164aeeda4
	github.com/matrix-org/go-sqlite3-js v0.0.0-20200522092705-bc8506ccbcf3
	github.com/matrix-org/gomatrix v0.0.0-20190528120928-7df988a63f26
	github.com/matrix-org/gomatrixserverlib v0.0.0-20200807145008-79c173b65786
	github.com/matrix-org/naffka v0.0.0-20200422140631-181f1ee7401f
	github.com/matrix-org/util v0.0.0-20200807132607-55161520e1d4
	github.com/mattn/go-sqlite3 v2.0.2+incompatible
	github.com/nfnt/resize v0.0.0-20160724205520-891127d8d1b5
	github.com/ngrok/sqlmw v0.0.0-20200129213757-d5c93a81bec6
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.4.1
	github.com/sirupsen/logrus v1.6.0
	github.com/tidwall/gjson v1.6.0
	github.com/tidwall/sjson v1.0.3
	github.com/uber-go/atomic v1.3.0 // indirect
	github.com/uber/jaeger-client-go v2.15.0+incompatible
	github.com/uber/jaeger-lib v1.5.0
	github.com/yggdrasil-network/yggdrasil-go v0.3.15-0.20200806125501-cd4685a3b4de
	go.uber.org/atomic v1.4.0
	golang.org/x/crypto v0.0.0-20200423211502-4bdfaf469ed5
	gopkg.in/h2non/bimg.v1 v1.0.18
	gopkg.in/yaml.v2 v2.3.0
)

go 1.13
