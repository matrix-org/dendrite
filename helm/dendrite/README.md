# dendrite

![Version: 0.10.8](https://img.shields.io/badge/Version-0.10.8-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.10.8](https://img.shields.io/badge/AppVersion-0.10.8-informational?style=flat-square)
Dendrite Matrix Homeserver

Status: **NOT PRODUCTION READY**

## About

This is a first try for a Helm Chart for the [Matrix](https://matrix.org) Homeserver [Dendrite](https://github.com/matrix-org/dendrite)

This chart creates a polylith, where every component is in its own deployment and requires a Postgres server aswell as a NATS JetStream server.

## Manual database creation

(You can skip this, if you're deploying the PostgreSQL dependency)

You'll need to create the following databases before starting Dendrite (see [install.md](https://github.com/matrix-org/dendrite/blob/master/docs/INSTALL.md#configuration)):

```postgres
create database dendrite_federationapi;
create database dendrite_mediaapi;
create database dendrite_roomserver;
create database dendrite_userapi_accounts;
create database dendrite_keyserver;
create database dendrite_syncapi;
```

or

```bash
for i in mediaapi syncapi roomserver federationapi keyserver userapi_accounts; do
    sudo -u postgres createdb -O dendrite dendrite_$i
done
```

## Usage with appservices

Create a folder `appservices` and place your configurations in there.  The configurations will be read and placed in a secret `dendrite-appservices-conf`.

## Source Code

* <https://github.com/matrix-org/dendrite>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clientapi.registration.disabled | bool | `true` | Disable registration |
| clientapi.registration.enable_registration_captcha | bool | `false` | enable reCAPTCHA registration |
| clientapi.registration.recaptcha_bypass_secret | string | `""` | reCAPTCHA bypass secret |
| clientapi.registration.recaptcha_private_key | string | `""` | reCAPTCHA private key |
| clientapi.registration.recaptcha_public_key | string | `""` | reCAPTCHA public key  |
| clientapi.registration.recaptcha_siteverify_api | string | `""` |  |
| clientapi.registration.shared_secret | string | `""` | If set, allows registration by anyone who knows the shared secret, regardless of whether registration is otherwise disabled. |
| configuration.cache.max_age | string | `"1h"` | The maximum amount of time that a cache entry can live for in memory before it will be evicted and/or refreshed from the database. Lower values result in easier admission of new cache entries but may also increase database load in comparison to higher values, so adjust conservatively. Higher values may make it harder for new items to make it into the cache, e.g. if new rooms suddenly become popular. |
| configuration.cache.max_size_estimated | string | `"1gb"` | The estimated maximum size for the global cache in bytes, or in terabytes, gigabytes, megabytes or kilobytes when the appropriate 'tb', 'gb', 'mb' or 'kb' suffix is specified. Note that this is not a hard limit, nor is it a memory limit for the entire process. A cache that is too small may ultimately provide little or no benefit. |
| configuration.database.conn_max_lifetime | int | `-1` | Default database maximum lifetime |
| configuration.database.host | string | `""` | Default database host |
| configuration.database.max_idle_conns | int | `2` | Default database maximum idle connections |
| configuration.database.max_open_conns | int | `100` | Default database maximum open connections |
| configuration.database.password | string | `""` | Default database password |
| configuration.database.user | string | `""` | Default database user |
| configuration.disable_federation | bool | `false` | Disable federation. Dendrite will not be able to make any outbound HTTP requests to other servers and the federation API will not be exposed. |
| configuration.dns_cache.cache_lifetime | string | `"10m"` | Duration for how long DNS cache items should be considered valid ([see time.ParseDuration](https://pkg.go.dev/time#ParseDuration) for more) |
| configuration.dns_cache.cache_size | int | `256` | Maximum number of entries to hold in the DNS cache |
| configuration.dns_cache.enabled | bool | `false` | Whether or not the DNS cache is enabled. |
| configuration.key_validity_period | string | `"168h0m0s"` |  |
| configuration.logging | list | [default dendrite config values](https://github.com/matrix-org/dendrite/blob/master/dendrite-config.yaml) | Default logging configuration |
| configuration.metrics.basic_auth.password | string | `"metrics"` | HTTP basic authentication password |
| configuration.metrics.basic_auth.user | string | `"metrics"` | HTTP basic authentication username |
| configuration.metrics.enabled | bool | `false` | Whether or not Prometheus metrics are enabled. |
| configuration.mscs | list | `[]` | Configuration for experimental MSC's. (Valid values are: msc2836 and msc2946) |
| configuration.profiling.enabled | bool | `false` | Enable pprof |
| configuration.profiling.port | int | `65432` | pprof port, if enabled |
| configuration.rate_limiting.cooloff_ms | int | `500` | Cooloff time in milliseconds |
| configuration.rate_limiting.enabled | bool | `true` | Enable rate limiting |
| configuration.rate_limiting.threshold | int | `5` | After how many requests a rate limit should be activated |
| configuration.servername | string | `""` | Servername for this Dendrite deployment |
| configuration.signing_key.create | bool | `true` | Create a new signing key, if not exists |
| configuration.signing_key.existingSecret | string | `""` | Use an existing secret |
| configuration.tracing | object | disabled | Default tracing configuration |
| configuration.trusted_third_party_id_servers | list | `["matrix.org","vector.im"]` | Lists of domains that the server will trust as identity servers to verify third party identifiers such as phone numbers and email addresses. |
| configuration.turn.turn_password | string | `""` | The TURN password |
| configuration.turn.turn_shared_secret | string | `""` |  |
| configuration.turn.turn_uris | list | `[]` |  |
| configuration.turn.turn_user_lifetime | string | `""` |  |
| configuration.turn.turn_username | string | `""` | The TURN username |
| configuration.well_known_client_name | string | `nil` | The server name to delegate client-server communications to, with optional port e.g. localhost:443 |
| configuration.well_known_server_name | string | `""` | The server name to delegate server-server communications to, with optional port e.g. localhost:443 |
| federationapi.disable_tls_validation | bool | `false` | Disable TLS validation |
| federationapi.prefer_direct_fetch | bool | `false` |  |
| federationapi.send_max_retries | int | `16` |  |
| image.name | string | `"ghcr.io/matrix-org/dendrite-monolith:v0.10.8"` | Docker repository/image to use |
| image.pullPolicy | string | `"IfNotPresent"` | Kubernetes pullPolicy |
| ingress.annotations | object | `{}` |  |
| ingress.enabled | bool | `false` | Create an ingress for a monolith deployment |
| ingress.hosts | list | `[]` |  |
| ingress.tls | list | `[]` |  |
| mediaapi.dynamic_thumbnails | bool | `false` |  |
| mediaapi.max_file_size_bytes | string | `"10485760"` | The max file size for uploaded media files |
| mediaapi.max_thumbnail_generators | int | `10` | The maximum number of simultaneous thumbnail generators to run. |
| mediaapi.thumbnail_sizes | list | [default dendrite config values](https://github.com/matrix-org/dendrite/blob/master/dendrite-config.yaml) | A list of thumbnail sizes to be generated for media content. |
| persistence.jetstream.capacity | string | `"5Gi"` |  |
| persistence.jetstream.existingClaim | string | `""` |  |
| persistence.media.capacity | string | `"10Gi"` |  |
| persistence.media.existingClaim | string | `""` |  |
| persistence.search.capacity | string | `"5Gi"` |  |
| persistence.search.existingClaim | string | `""` |  |
| persistence.storageClass | string | `"local-path"` |  |
| postgresql.auth.database | string | `"dendrite"` |  |
| postgresql.auth.password | string | `"changeme"` |  |
| postgresql.auth.username | string | `"dendrite"` |  |
| postgresql.enabled | bool | See value.yaml | Enable and configure postgres as the database for dendrite. |
| postgresql.image.repository | string | `"bitnami/postgresql"` |  |
| postgresql.image.tag | string | `"14.4.0"` |  |
| postgresql.initdbScripts."create_db.sh" | string | creates the required databases | Create databases when first creating a PostgreSQL Server |
| postgresql.persistence.enabled | bool | `false` |  |
| resources | object | sets some sane default values | Default resource requests/limits. This can be set individually for each component, see mediaapi |
| syncapi.real_ip_header | string | `"X-Real-IP"` | This option controls which HTTP header to inspect to find the real remote IP address of the client. This is likely required if Dendrite is running behind a reverse proxy server. |
| syncapi.search | object | `{"enabled":false,"language":"en"}` | Configuration for the full-text search engine. |
| syncapi.search.enabled | bool | `false` | Whether or not search is enabled. |
| syncapi.search.language | string | `"en"` | The language most likely to be used on the server - used when indexing, to ensure the returned results match expectations. A full list of possible languages can be found at https://github.com/blevesearch/bleve/tree/master/analysis/lang |
