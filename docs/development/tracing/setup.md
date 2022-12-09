---
title: Setup
parent: OpenTracing
grand_parent: Development
permalink: /development/opentracing/setup
---

# OpenTracing Setup

Dendrite uses [Jaeger](https://www.jaegertracing.io/) for tracing between microservices.
Tracing shows the nesting of logical spans which provides visibility on how the microservices interact.
This document explains how to set up Jaeger locally on a single machine.

## Set up the Jaeger backend

The [easiest way](https://www.jaegertracing.io/docs/1.18/getting-started/) is to use the all-in-one Docker image:

```
$ docker run -d --name jaeger \
  -e COLLECTOR_ZIPKIN_HTTP_PORT=9411 \
  -p 5775:5775/udp \
  -p 6831:6831/udp \
  -p 6832:6832/udp \
  -p 5778:5778 \
  -p 16686:16686 \
  -p 14268:14268 \
  -p 14250:14250 \
  -p 9411:9411 \
  jaegertracing/all-in-one:1.18
```

## Configuring Dendrite to talk to Jaeger

Modify your config to look like: (this will send every single span to Jaeger which will be slow on large instances, but for local testing it's fine)

```
tracing:
  enabled: true
  jaeger:
    serviceName: "dendrite"
    disabled: false
    rpc_metrics: true
    tags: []
    sampler:
      type: const
      param: 1
```

then run the monolith server with `--api true` to use polylith components which do tracing spans:

```
./dendrite-monolith-server --tls-cert server.crt --tls-key server.key --config dendrite.yaml --api true
```

## Checking traces

Visit <http://localhost:16686> to see traces under `DendriteMonolith`.
