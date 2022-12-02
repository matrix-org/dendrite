---
title: Setting up the domain
parent: Installation
nav_order: 2
permalink: /installation/domainname
---

# Setting up the domain

Every Matrix server deployment requires a server name which uniquely identifies it. For
example, if you are using the server name `example.com`, then your users will have usernames
that take the format `@user:example.com`.

For federation to work, the server name must be resolvable by other homeservers on the internet
— that is, the domain must be registered and properly configured with the relevant DNS records.

Matrix servers usually discover each other when federating using the following methods:

1. If a well-known delegation exists on `example.com`, use the domain and port from the
   well-known file to connect to the remote homeserver;
2. If a DNS SRV delegation exists on `example.com`, use the IP address and port from the DNS SRV
   record to connect to the remote homeserver;
3. If neither well-known or DNS SRV delegation are configured, attempt to connect to the remote
   homeserver by connecting to `example.com` port TCP/8448 using HTTPS.

The exact details of how server name resolution works can be found in
[the spec](https://spec.matrix.org/v1.3/server-server-api/#resolving-server-names).

## TLS certificates

Matrix federation requires that valid TLS certificates are present on the domain. You must
obtain certificates from a publicly-trusted certificate authority (CA). [Let's Encrypt](https://letsencrypt.org)
is a popular choice of CA because the certificates are publicly-trusted, free, and automated
via the ACME protocol. (Self-signed certificates are not suitable for federation and will typically
not be accepted by other homeservers.)

Automating the renewal of TLS certificates is best practice. There are many tools for this,
but the simplest way to achieve TLS automation is to have your reverse proxy do it for you.
[Caddy](https://caddyserver.com) is recommended as a production-grade reverse proxy with
automatic TLS which is commonly used in front of Dendrite. It obtains and renews TLS certificates
automatically and by default as long as your domain name is pointed at your server first.
Although the finer details of [configuring Caddy](https://caddyserver.com/docs/) is not described
here, in general, you must reverse proxy all `/_matrix` paths to your Dendrite server. For example,
with Caddy:

```
reverse_proxy /_matrix/* localhost:8008
```

It is possible for the reverse proxy to listen on the standard HTTPS port TCP/443 so long as your
domain delegation is configured to point to port TCP/443.

## Delegation

Delegation allows you to specify the server name and port that your Dendrite installation is
reachable at, or to host the Dendrite server at a different server name to the domain that
is being delegated.

For example, if your Dendrite installation is actually reachable at `matrix.example.com` port 8448,
you will be able to delegate from `example.com` to `matrix.example.com` so that your users will have
`@user:example.com` user names instead of `@user:matrix.example.com` usernames.

Delegation can be performed in one of two ways:

* **Well-known delegation (preferred)**: A well-known text file is served over HTTPS on the domain
  name that you want to use, pointing to your server on `matrix.example.com` port 8448;
* **DNS SRV delegation (not recommended)**: See the SRV delegation section below for details.

If you are using a reverse proxy to forward `/_matrix` to Dendrite, your well-known or delegation
must refer to the hostname and port that the reverse proxy is listening on instead.

## Well-known delegation

Using well-known delegation requires that you are running a web server at `example.com` which
is listening on the standard HTTPS port TCP/443.

Assuming that your Dendrite installation is listening for HTTPS connections at `matrix.example.com`
on port 8448, the delegation file must be served at `https://example.com/.well-known/matrix/server`
and contain the following JSON document:

```json
{
    "m.server": "matrix.example.com:8448"
}
```

For example, this can be done with the following Caddy config:

```
handle /.well-known/matrix/server {
	header Content-Type application/json
	header Access-Control-Allow-Origin *
	respond `{"m.server": "matrix.example.com:8448"}`
}

handle /.well-known/matrix/client {
	header Content-Type application/json
	header Access-Control-Allow-Origin *
	respond `{"m.homeserver": {"base_url": "https://matrix.example.com:8448"}}`
}
```

You can also serve `.well-known` with Dendrite itself by setting the `well_known_server_name` config
option to the value you want for `m.server`. This is primarily useful if Dendrite is exposed on
`example.com:443` and you don't want to set up a separate webserver just for serving the `.well-known`
file.

```yaml
global:
...
   well_known_server_name: "example.com:443"
```

## DNS SRV delegation

This method is not recommended, as the behavior of SRV records in Matrix is rather unintuitive:
SRV records will only change the IP address and port that other servers connect to, they won't
affect the domain name. In technical terms, the `Host` header and TLS SNI of federation requests
will still be `example.com` even if the SRV record points at `matrix.example.com`.

In practice, this means that the server must be configured with valid TLS certificates for
`example.com`, rather than `matrix.example.com` as one might intuitively expect. If there's a
reverse proxy in between, the proxy configuration must be written as if it's `example.com`, as the
proxy will never see the name `matrix.example.com` in incoming requests.

This behavior also means that if `example.com` and `matrix.example.com` point at the same IP
address, there is no reason to have a SRV record pointing at `matrix.example.com`. It can still
be used to change the port number, but it won't do anything else.

If you understand how SRV records work and still want to use them, the service name is `_matrix` and
the protocol is `_tcp`.
