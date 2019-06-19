module github.com/matrix-org/dendrite

require (
	cloud.google.com/go v0.40.0 // indirect
	github.com/OpenPeeDeeP/depguard v0.0.0-20181229194401-1f388ab2d810 // indirect
	github.com/Shopify/sarama v0.0.0-20170127151855-574d3147eee3
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/alecthomas/gometalinter v2.0.2+incompatible
	github.com/alecthomas/units v0.0.0-20151022065526-2efee857e7cf
	github.com/apache/thrift v0.0.0-20161221203622-b2a4d4ae21c7
	github.com/beorn7/perks v1.0.0
	github.com/codahale/hdrhistogram v0.0.0-20161010025455-3a0bb77429bd
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/etcd v3.3.13+incompatible // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190617083831-1652836e9bdc // indirect
	github.com/crossdock/crossdock-go v0.0.0-20160816171116-049aabb0122b
	github.com/davecgh/go-spew v1.1.1
	github.com/eapache/go-resiliency v0.0.0-20160104191539-b86b1ec0dd42
	github.com/eapache/go-xerial-snappy v0.0.0-20160609142408-bb955e01b934
	github.com/eapache/queue v1.1.0
	github.com/fatih/color v1.7.0 // indirect
	github.com/go-critic/go-critic v0.3.4 // indirect
	github.com/go-logfmt/logfmt v0.4.0 // indirect
	github.com/go-ole/go-ole v1.2.4 // indirect
	github.com/golang/mock v1.3.1 // indirect
	github.com/golang/protobuf v1.3.1
	github.com/golang/snappy v0.0.0-20170119014723-7db9049039a0
	github.com/golangci/errcheck v0.0.0-20181223084120-ef45e06d44b6 // indirect
	github.com/golangci/go-tools v0.0.0-20190124090046-35a9f45a5db0 // indirect
	github.com/golangci/gocyclo v0.0.0-20180528144436-0a533e8fa43d // indirect
	github.com/golangci/gofmt v0.0.0-20181222123516-0b8337e80d98 // indirect
	github.com/golangci/golangci-lint v1.17.1 // indirect
	github.com/golangci/gosec v0.0.0-20180901114220-8afd9cbb6cfb // indirect
	github.com/golangci/lint-1 v0.0.0-20181222135242-d2cdd8c08219 // indirect
	github.com/golangci/revgrep v0.0.0-20180812185044-276a5c0a1039 // indirect
	github.com/google/pprof v0.0.0-20190515194954-54271f7e092f // indirect
	github.com/google/shlex v0.0.0-20150127133951-6f45313302b9
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	github.com/gorilla/context v1.1.1
	github.com/gorilla/mux v1.3.0
	github.com/gostaticanalysis/analysisutil v0.0.0-20190329151158-56bca42c7635 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.9.1 // indirect
	github.com/jaegertracing/jaeger-client-go v0.0.0-20170921145708-3ad49a1d839b
	github.com/jaegertracing/jaeger-lib v0.0.0-20170920222118-21a3da6d66fe
	github.com/kisielk/errcheck v1.2.0 // indirect
	github.com/klauspost/compress v1.7.0 // indirect
	github.com/klauspost/cpuid v1.2.1 // indirect
	github.com/klauspost/crc32 v0.0.0-20161016154125-cb6bfca970f6
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kr/pty v1.1.5 // indirect
	github.com/lib/pq v0.0.0-20170918175043-23da1db4f16d
	github.com/logrusorgru/aurora v0.0.0-20190428105938-cea283e61946 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/matrix-org/dugong v0.0.0-20171220115018-ea0a4690a0d5
	github.com/matrix-org/gomatrix v0.0.0-20171003113848-a7fc80c8060c
	github.com/matrix-org/gomatrixserverlib v0.0.0-20181109104322-1c2cbc0872f0
	github.com/matrix-org/naffka v0.0.0-20171115094957-662bfd0841d0
	github.com/matrix-org/util v0.0.0-20171013132526-8b1c8ab81986
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/mozilla/tls-observatory v0.0.0-20180409132520-8791a200eb40 // indirect
	github.com/nbutton23/zxcvbn-go v0.0.0-20180912185939-ae427f1e4c1d // indirect
	github.com/nfnt/resize v0.0.0-20160724205520-891127d8d1b5
	github.com/nicksnyder/go-i18n v1.8.1
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/opentracing/opentracing-go v0.0.0-20170806192116-8ebe5d4e236e
	github.com/pelletier/go-toml v1.4.0
	github.com/pierrec/lz4 v0.0.0-20161206202305-5c9560bfa9ac
	github.com/pierrec/xxHash v0.0.0-20160112165351-5a004441f897
	github.com/pkg/errors v0.8.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v1.0.0
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90
	github.com/prometheus/common v0.4.1
	github.com/prometheus/procfs v0.0.2
	github.com/rcrowley/go-metrics v0.0.0-20161128210544-1f30fe9094a5
	github.com/rogpeppe/fastuuid v1.1.0 // indirect
	github.com/russross/blackfriday v2.0.0+incompatible // indirect
	github.com/ryanuber/go-glob v0.0.0-20170128012129-256dc444b735 // indirect
	github.com/shirou/gopsutil v2.18.12+incompatible // indirect
	github.com/shurcooL/go v0.0.0-20190330031554-6713ea532688 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/afero v1.2.2 // indirect
	github.com/spf13/cobra v0.0.5 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.4.0 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/tidwall/gjson v1.0.2
	github.com/tidwall/match v0.0.0-20171002075945-1731857f09b1
	github.com/tidwall/sjson v1.0.0
	github.com/uber-go/atomic v1.3.0
	github.com/uber/jaeger-client-go v2.15.0+incompatible
	github.com/uber/jaeger-lib v1.5.0
	github.com/uber/tchannel-go v0.0.0-20170927010734-b3e26487e291
	github.com/ugorji/go v1.1.5-pre // indirect
	github.com/valyala/fasthttp v1.3.0 // indirect
	go.etcd.io/bbolt v1.3.3 // indirect
	go.opencensus.io v0.22.0 // indirect
	go.uber.org/atomic v1.4.0
	go.uber.org/multierr v1.1.0
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20190611184440-5c40567a22f8
	golang.org/x/exp v0.0.0-20190510132918-efd6b22b2522 // indirect
	golang.org/x/image v0.0.0-20190616094056-33659d3de4f5 // indirect
	golang.org/x/mobile v0.0.0-20190607214518-6fa95d984e88 // indirect
	golang.org/x/mod v0.1.0 // indirect
	golang.org/x/net v0.0.0-20190613194153-d28f0bde5980
	golang.org/x/sys v0.0.0-20190618155005-516e3c20635f
	golang.org/x/tools v0.0.0-20190614205625-5aca471b1d59 // indirect
	google.golang.org/appengine v1.6.1 // indirect
	google.golang.org/genproto v0.0.0-20190611190212-a7e196e89fd3 // indirect
	google.golang.org/grpc v1.21.1 // indirect
	gopkg.in/Shopify/sarama.v1 v1.11.0
	gopkg.in/airbrake/gobrake.v2 v2.0.9
	gopkg.in/alecthomas/kingpin.v3-unstable v3.0.0-20170727041045-23bcc3c4eae3
	gopkg.in/gemnasium/logrus-airbrake-hook.v2 v2.1.2
	gopkg.in/h2non/bimg.v1 v1.0.18
	gopkg.in/macaroon.v2 v2.0.0
	gopkg.in/yaml.v2 v2.2.2
	honnef.co/go/tools v0.0.0-20190614002413-cb51c254f01b // indirect
	mvdan.cc/unparam v0.0.0-20190310220240-1b9ccfa71afe // indirect
	sourcegraph.com/sourcegraph/go-diff v0.5.1-0.20190210232911-dee78e514455 // indirect
	sourcegraph.com/sqs/pbtypes v1.0.0 // indirect
)
