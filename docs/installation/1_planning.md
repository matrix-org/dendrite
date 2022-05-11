---
title: Planning your installation
parent: Installation
nav_order: 1
permalink: /installation/planning
---

# Planning your installation

## Modes

Dendrite can be run in one of two configurations:

* **Monolith mode**: All components run in the same process. In this mode,
   it is possible to run an in-process NATS Server instead of running a standalone deployment.
   This will usually be the preferred model for low-to-mid volume deployments, providing the best
   balance between performance and resource usage.

* **Polylith mode**: A cluster of individual components running in their own processes, dealing
  with different aspects of the Matrix protocol. Components communicate with each other using
  internal HTTP APIs and NATS Server. This will almost certainly be the preferred model for very
  large deployments but scalability comes with a cost. API calls are expensive and therefore a
  polylith deployment may end up using disproportionately more resources for a smaller number of
  users compared to a monolith deployment.

At present, we **recommend monolith mode deployments** in all cases.

## Databases

Dendrite can run with either a PostgreSQL or a SQLite backend. There are considerable tradeoffs
to consider:

* **PostgreSQL**: Needs to run separately to Dendrite, needs to be installed and configured separately
  and and will use more resources over all, but will be **considerably faster** than SQLite. PostgreSQL
  has much better write concurrency which will allow Dendrite to process more tasks in parallel. This
  will be necessary for federated deployments to perform adequately.

* **SQLite**: Built into Dendrite, therefore no separate database engine is necessary and is quite
  a bit easier to set up, but will be much slower than PostgreSQL in most cases. SQLite only allows a
  single writer on a database at a given time, which will significantly restrict Dendrite's ability
  to process multiple tasks in parallel.

At this time, we **recommend the PostgreSQL database engine** for all production deployments.

## Requirements

Dendrite will run on Linux, macOS and Windows Server. It should also run fine on variants
of BSD such as FreeBSD and OpenBSD. We have not tested Dendrite on AIX, Solaris, Plan 9 or z/OS —
your mileage may vary with these platforms.

It is difficult to state explicitly the amount of CPU, RAM or disk space that a Dendrite
installation will need, as this varies considerably based on a number of factors. In particular:

* The number of users using the server;
* The number of rooms that the server is joined to — federated rooms in particular will typically
  use more resources than rooms with only local users;
* The complexity of rooms that the server is joined to — rooms with more members coming and
  going will typically be of a much higher complexity.

Some tasks are more expensive than others, such as joining rooms over federation, running state
resolution or sending messages into very large federated rooms with lots of remote users. Therefore
you should plan accordingly and ensure that you have enough resources available to endure spikes
in CPU or RAM usage, as these may be considerably higher than the idle resource usage.

At an absolute minimum, Dendrite will expect 1GB RAM. For a comfortable day-to-day deployment
which can participate in federated rooms for a number of local users, be prepared to assign 2-4
CPU cores and 8GB RAM — more if your user count increases.

If you are running PostgreSQL on the same machine, allow extra headroom for this too, as the
database engine will also have CPU and RAM requirements of its own. Running too many heavy
services on the same machine may result in resource starvation and processes may end up being
killed by the operating system if they try to use too much memory.

## Dependencies

In order to install Dendrite, you will need to satisfy the following dependencies.

### Go

At this time, Dendrite supports being built with Go 1.16 or later. We do not support building
Dendrite with older versions of Go than this. If you are installing Go using a package manager,
you should check (by running `go version`) that you are using a suitable version before you start.

### PostgreSQL

If using the PostgreSQL database engine, you should install PostgreSQL 12 or later.

### NATS Server

Monolith deployments come with a built-in [NATS Server](https://github.com/nats-io/nats-server) and
therefore do not need this to be manually installed. If you are planning a monolith installation, you
do not need to do anything.

Polylith deployments, however, currently need a standalone NATS Server installation with JetStream
enabled.

To do so, follow the [NATS Server installation instructions](https://docs.nats.io/running-a-nats-service/introduction/installation) and then [start your NATS deployment](https://docs.nats.io/running-a-nats-service/introduction/running). JetStream must be enabled, either by passing the `-js` flag to `nats-server`,
or by specifying the `store_dir` option in the the `jetstream` configuration.

### Reverse proxy (polylith deployments)

Polylith deployments require a reverse proxy, such as [NGINX](https://www.nginx.com) or
[HAProxy](http://www.haproxy.org). Configuring those is not covered in this documentation,
although a [sample configuration for NGINX](https://github.com/matrix-org/dendrite/blob/main/docs/nginx/polylith-sample.conf)
is provided.

### Windows

Finally, if you want to build Dendrite on Windows, you will need need `gcc` in the path. The best
way to achieve this is by installing and building Dendrite under [MinGW-w64](https://www.mingw-w64.org/).
