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

Matrix servers discover each other when federating using the following methods:

1. If a well-known delegation exists on `example.com`, use the path server from the
   well-known file to connect to the remote homeserver;
2. If a DNS SRV delegation exists on `example.com`, use the hostname and port from the DNS SRV
   record to connect to the remote homeserver;
3. If neither well-known or DNS SRV delegation are configured, attempt to connect to the remote
   homeserver by connecting to `example.com` port TCP/8448 using HTTPS.

## TLS certificates

Matrix federation requires that valid TLS certificates are present on the domain. You must
obtain certificates from a publicly accepted Certificate Authority (CA). [LetsEncrypt](https://letsencrypt.org)
is an example of such a CA that can be used. Self-signed certificates are not suitable for
federation and will typically not be accepted by other homeservers.

A common practice to help ease the management of certificates is to install a reverse proxy in
front of Dendrite which manages the TLS certificates and HTTPS proxying itself. Software such as
[NGINX](https://www.nginx.com) and [HAProxy](http://www.haproxy.org) can be used for the task.
Although the finer details of configuring these are not described here, you must reverse proxy
all `/_matrix` paths to your Dendrite server.

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

* **Well-known delegation**: A well-known text file is served over HTTPS on the domain name
  that you want to use, pointing to your server on `matrix.example.com` port 8448;
* **DNS SRV delegation**: A DNS SRV record is created on the domain name that you want to
  use, pointing to your server on `matrix.example.com` port TCP/8448.

If you are using a reverse proxy to forward `/_matrix` to Dendrite, your well-known or DNS SRV
delegation must refer to the hostname and port that the reverse proxy is listening on instead.

Well-known delegation is typically easier to set up and usually preferred. However, you can use
either or both methods to delegate. If you configure both methods of delegation, it is important
that they both agree and refer to the same hostname and port.

## Well-known delegation

Using well-known delegation requires that you are running a web server at `example.com` which
is listening on the standard HTTPS port TCP/443.

Assuming that your Dendrite installation is listening for HTTPS connections at `matrix.example.com`
on port 8448, the delegation file must be served at `https://example.com/.well-known/matrix/server`
and contain the following JSON document:

```json
{
    "m.server": "https://matrix.example.com:8448"
}
```

## DNS SRV delegation

Using DNS SRV delegation requires creating DNS SRV records on the `example.com` zone which
refer to your Dendrite installation.

Assuming that your Dendrite installation is listening for HTTPS connections at `matrix.example.com`
port 8448, the DNS SRV record must have the following fields:

* Name: `@` (or whichever term your DNS provider uses to signal the root)
* Service: `_matrix`
* Protocol: `_tcp`
* Port: `8448`
* Target: `matrix.example.com`
