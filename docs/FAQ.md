# Frequently Asked Questions

### Is Dendrite stable?

Mostly, although there are still bugs and missing features. If you are a confident power user and you are happy to spend some time debugging things when they go wrong, then please try out Dendrite. If you are a community, organisation or business that demands stability and uptime, then Dendrite is not for you yet - please install Synapse instead.

### Is Dendrite feature-complete?

No, although a good portion of the Matrix specification has been implemented. Mostly missing are client features - see the readme at the root of the repository for more information.

### Is there a migration path from Synapse to Dendrite?

No, not at present. There will be in the future when Dendrite reaches version 1.0.

### Can I use Dendrite with an existing Synapse database?

No, Dendrite has a very different database schema to Synapse and the two are not interchangeable.

### Should I run a monolith or a polylith deployment?

Monolith deployments are always preferred where possible, and at this time, are far better tested than polylith deployments are. The only reason to consider a polylith deployment is if you wish to run different Dendrite components on separate physical machines.

### I've installed Dendrite but federation isn't working

Check the [Federation Tester](https://federationtester.matrix.org). You need at least:

* A valid DNS name
* A valid TLS certificate for that DNS name
* Either DNS SRV records or well-known files

### Does Dendrite work with my favourite client?

It should do, although we are aware of some minor issues:

* **Element Android**: registration does not work, but logging in with an existing account does
* **Hydrogen**: occasionally sync can fail due to gaps in the `since` parameter, but clearing the cache fixes this

### Does Dendrite support push notifications?

Yes, we have experimental support for push notifications. Configure them in the usual way in your Matrix client.

### Does Dendrite support application services/bridges?

Possibly - Dendrite does have some application service support but it is not well tested. Please let us know by raising a GitHub issue if you try it and run into problems.

Bridges known to work (as of v0.5.1):
- [Telegram](https://docs.mau.fi/bridges/python/telegram/index.html)
- [WhatsApp](https://docs.mau.fi/bridges/go/whatsapp/index.html)
- [Signal](https://docs.mau.fi/bridges/python/signal/index.html)
- [probably all other mautrix bridges](https://docs.mau.fi/bridges/)

Remember to add the config file(s) to the `app_service_api` [config](https://github.com/matrix-org/dendrite/blob/de38be469a23813921d01bef3e14e95faab2a59e/dendrite-config.yaml#L130-L131).

### Is it possible to prevent communication with the outside world?

Yes, you can do this by disabling federation - set `disable_federation` to `true` in the `global` section of the Dendrite configuration file. 

### Should I use PostgreSQL or SQLite for my databases?

Please use PostgreSQL wherever possible, especially if you are planning to run a homeserver that caters to more than a couple of users. 

### Dendrite is using a lot of CPU

Generally speaking, you should expect to see some CPU spikes, particularly if you are joining or participating in large rooms. However, constant/sustained high CPU usage is not expected - if you are experiencing that, please join `#dendrite-dev:matrix.org` and let us know, or file a GitHub issue.

### Dendrite is using a lot of RAM

A lot of users report that Dendrite is using a lot of RAM, sometimes even gigabytes of it. This is usually due to Go's allocator behaviour, which tries to hold onto allocated memory until the operating system wants to reclaim it for something else. This can make the memory usage look significantly inflated in tools like `top`/`htop` when actually most of that memory is not really in use at all.

If you want to prevent this behaviour so that the Go runtime releases memory normally, start Dendrite using the `GODEBUG=madvdontneed=1` environment variable. It is also expected that the allocator behaviour will be changed again in Go 1.16 so that it does not hold onto memory unnecessarily in this way.

If you are running with `GODEBUG=madvdontneed=1` and still see hugely inflated memory usage then that's quite possibly a bug - please join `#dendrite-dev:matrix.org` and let us know, or file a GitHub issue.

### Dendrite is running out of PostgreSQL database connections

You may need to revisit the connection limit of your PostgreSQL server and/or make changes to the `max_connections` lines in your Dendrite configuration. Be aware that each Dendrite component opens its own database connections and has its own connection limit, even in monolith mode!
