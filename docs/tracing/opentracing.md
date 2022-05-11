---
title: OpenTracing
has_children: true
parent: Development
permalink: /development/opentracing
---

# OpenTracing

Dendrite extensively uses the [opentracing.io](http://opentracing.io) framework
to trace work across the different logical components.

At its most basic opentracing tracks "spans" of work; recording start and end
times as well as any parent span that caused the piece of work.

A typical example would be a new span being created on an incoming request that
finishes when the response is sent. When the code needs to hit out to a
different component a new span is created with the initial span as its parent.
This would end up looking roughly like:

```
Received request                             Sent response
       |<───────────────────────────────────────>|
                 |<────────────────────>|
            RPC call            RPC call returns
```

This is useful to see where the time is being spent processing a request on a
component. However, opentracing allows tracking of spans across components. This
makes it possible to see exactly what work goes into processing a request:

```
Component 1       |<─────────────────── HTTP ────────────────────>|
                     |<──────────────── RPC ─────────────────>|
Component 2                |<─ SQL ─>|     |<── RPC ───>|
Component 3                                  |<─ SQL ─>|
```

This is achieved by serializing span information during all communication
between components. For HTTP requests, this is achieved by the sender
serializing the span into a HTTP header, and the receiver deserializing the span
on receipt. (Generally a new span is then immediately created with the
deserialized span as the parent).

A collection of spans that are related is called a trace.

Spans are passed through the code via contexts, rather than manually. It is
therefore important that all spans that are created are immediately added to the
current context. Thankfully the opentracing library gives helper functions for
doing this:

```golang
span, ctx := opentracing.StartSpanFromContext(ctx, spanName)
defer span.Finish()
```

This will create a new span, adding any span already in `ctx` as a parent to the
new span.

Adding Information
------------------

Opentracing allows adding information to a trace via three mechanisms:

- "tags" ─ A span can be tagged with a key/value pair. This is typically
  information that relates to the span, e.g. for spans created for incoming HTTP
  requests could include the request path and response codes as tags, spans for
  SQL could include the query being executed.
- "logs" ─ Key/value pairs can be looged at a particular instance in a trace.
  This can be useful to log e.g. any errors that happen.
- "baggage" ─ Arbitrary key/value pairs can be added to a span to which all
  child spans have access. Baggage isn't saved and so isn't available when
  inspecting the traces, but can be used to add context to logs or tags in child
  spans.

See
[specification.md](https://github.com/opentracing/specification/blob/master/specification.md)
for some of the common tags and log fields used.

Span Relationships
------------------

Spans can be related to each other. The most common relation is `childOf`, which
indicates the child span somehow depends on the parent span ─ typically the
parent span cannot complete until all child spans are completed.

A second relation type is `followsFrom`, where the parent has no dependence on
the child span. This usually indicates some sort of fire and forget behaviour,
e.g. adding a message to a pipeline or inserting into a kafka topic.

Jaeger
------

Opentracing is just a framework. We use
[jaeger](https://github.com/jaegertracing/jaeger) as the actual implementation.

Jaeger is responsible for recording, sending and saving traces, as well as
giving a UI for viewing and interacting with traces.

To enable jaeger a `Tracer` object must be instansiated from the config (as well
as having a jaeger server running somewhere, usually locally). A `Tracer` does
several things:

- Decides which traces to save and send to the server. There are multiple
  schemes for doing this, with a simple example being to save a certain fraction
  of traces.
- Communicating with the jaeger backend. If not explicitly specified uses the
  default port on localhost.
- Associates a service name to all spans created by the tracer. This service
  name equates to a logical component, e.g. spans created by clientapi will have
  a different service name than ones created by the syncapi. Database access
  will also typically use a different service name.

  This means that there is a tracer per service name/component.
