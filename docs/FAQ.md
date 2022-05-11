---
title: FAQ
nav_order: 1
permalink: /faq
---

# FAQ

## Is Dendrite stable?

Mostly, although there are still bugs and missing features. If you are a confident power user and you are happy to spend some time debugging things when they go wrong, then please try out Dendrite. If you are a community, organisation or business that demands stability and uptime, then Dendrite is not for you yet - please install Synapse instead.

## Is Dendrite feature-complete?

No, although a good portion of the Matrix specification has been implemented. Mostly missing are client features - see the readme at the root of the repository for more information.

## Is there a migration path from Synapse to Dendrite?

No, not at present. There will be in the future when Dendrite reaches version 1.0. For now it is not
possible to migrate an existing Synapse deployment to Dendrite.

## Can I use Dendrite with an existing Synapse database?

No, Dendrite has a very different database schema to Synapse and the two are not interchangeable.

## Should I run a monolith or a polylith deployment?

Monolith deployments are always preferred where possible, and at this time, are far better tested than polylith deployments are. The only reason to consider a polylith deployment is if you wish to run different Dendrite components on separate physical machines, but this is an advanced configuration which we don't
recommend.

## I've installed Dendrite but federation isn't working

Check the [Federation Tester](https://federationtester.matrix.org). You need at least:

* A valid DNS name
* A valid TLS certificate for that DNS name
* Either DNS SRV records or well-known files

## Does Dendrite work with my favourite client?

It should do, although we are aware of some minor issues:

* **Element Android**: registration does not work, but logging in with an existing account does
* **Hydrogen**: occasionally sync can fail due to gaps in the `since` parameter, but clearing the cache fixes this

## Does Dendrite support push notifications?

Yes, we have experimental support for push notifications. Configure them in the usual way in your Matrix client.

## Does Dendrite support application services/bridges?

Possibly - Dendrite does have some application service support but it is not well tested. Please let us know by raising a GitHub issue if you try it and run into problems.

Bridges known to work (as of v0.5.1):

* [Telegram](https://docs.mau.fi/bridges/python/telegram/index.html)
* [WhatsApp](https://docs.mau.fi/bridges/go/whatsapp/index.html)
* [Signal](https://docs.mau.fi/bridges/python/signal/index.html)
* [probably all other mautrix bridges](https://docs.mau.fi/bridges/)

Remember to add the config file(s) to the `app_service_api` [config](https://github.com/matrix-org/dendrite/blob/de38be469a23813921d01bef3e14e95faab2a59e/dendrite-config.yaml#L130-L131).

## Is it possible to prevent communication with the outside world?

Yes, you can do this by disabling federation - set `disable_federation` to `true` in the `global` section of the Dendrite configuration file.

## Should I use PostgreSQL or SQLite for my databases?

Please use PostgreSQL wherever possible, especially if you are planning to run a homeserver that caters to more than a couple of users.

## Dendrite is using a lot of CPU

Generally speaking, you should expect to see some CPU spikes, particularly if you are joining or participating in large rooms. However, constant/sustained high CPU usage is not expected - if you are experiencing that, please join `#dendrite-dev:matrix.org` and let us know what you were doing when the
CPU usage shot up, or file a GitHub issue. If you can take a [CPU profile](PROFILING.md) then that would
be a huge help too, as that will help us to understand where the CPU time is going.

## Dendrite is using a lot of RAM

As above with CPU usage, some memory spikes are expected if Dendrite is doing particularly heavy work
at a given instant. However, if it is using more RAM than you expect for a long time, that's probably
not expected. Join `#dendrite-dev:matrix.org` and let us know what you were doing when the memory usage
ballooned, or file a GitHub issue if you can. If you can take a [memory profile](PROFILING.md) then that
would be a huge help too, as that will help us to understand where the memory usage is happening.

## Dendrite is running out of PostgreSQL database connections

You may need to revisit the connection limit of your PostgreSQL server and/or make changes to the `max_connections` lines in your Dendrite configuration. Be aware that each Dendrite component opens its own database connections and has its own connection limit, even in monolith mode!

## What is being reported when enabling anonymous stats?

If anonymous stats reporting is enabled, the following data is send to the defined endpoint.

```json
{
    "cpu_average": 0,
    "daily_active_users": 97,
    "daily_e2ee_messages": 0,
    "daily_messages": 0,
    "daily_sent_e2ee_messages": 0,
    "daily_sent_messages": 0,
    "daily_user_type_bridged": 2,
    "daily_user_type_native": 97,
    "database_engine": "Postgres",
    "database_server_version": "11.14 (Debian 11.14-0+deb10u1)",
    "federation_disabled": false,
    "go_arch": "amd64",
    "go_os": "linux",
    "go_version": "go1.16.13",
    "homeserver": "localhost:8800",
    "log_level": "trace",
    "memory_rss": 93452,
    "monolith": true,
    "monthly_active_users": 97,
    "nats_embedded": true,
    "nats_in_memory": true,
    "num_cpu": 8,
    "num_go_routine": 203,
    "r30v2_users_all": 0,
    "r30v2_users_android": 0,
    "r30v2_users_electron": 0,
    "r30v2_users_ios": 0,
    "r30v2_users_web": 0,
    "timestamp": 1651741851,
    "total_nonbridged_users": 97,
    "total_room_count": 0,
    "total_users": 99,
    "uptime_seconds": 30,
    "version": "0.8.2"
}
```
