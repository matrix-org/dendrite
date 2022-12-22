# dendrite

![Version: 0.10.8](https://img.shields.io/badge/Version-0.10.8-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.10.8](https://img.shields.io/badge/AppVersion-0.10.8-informational?style=flat-square)
Dendrite Matrix Homeserver

Status: **NOT PRODUCTION READY**

## About

This chart creates a monolith deployment, including an optionally enabled PostgreSQL dependency to connect to.

## Manual database creation

(You can skip this, if you're deploying the PostgreSQL dependency)

You'll need to create the following database before starting Dendrite (see [installation](https://matrix-org.github.io/dendrite/installation/database#single-database-creation)):

```postgres
create database dendrite
```

or

```bash
sudo -u postgres createdb -O dendrite -E UTF-8 dendrite
```

## Usage with appservices

Create a folder `appservices` and place your configurations in there.  The configurations will be read and placed in a secret `dendrite-appservices-conf`.

## Source Code

* <https://github.com/matrix-org/dendrite>
## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.bitnami.com/bitnami | postgresql | 11.6.21 |
## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clientapi.enable_registration_captcha | bool | `false` | enable reCAPTCHA registration |
| clientapi.guests_disabled | bool | `true` |  |
| clientapi.rate_limiting.cooloff_ms | int | `500` | Cooloff time in milliseconds |
| clientapi.rate_limiting.enabled | bool | `true` | Enable rate limiting |
| clientapi.rate_limiting.exempt_user_ids | string | `nil` | Users which should be exempt from rate limiting |
| clientapi.rate_limiting.threshold | int | `20` | After how many requests a rate limit should be activated |
| clientapi.recaptcha_bypass_secret | string | `""` | reCAPTCHA bypass secret |
| clientapi.recaptcha_private_key | string | `""` | reCAPTCHA private key |
| clientapi.recaptcha_public_key | string | `""` | reCAPTCHA public key |
| clientapi.recaptcha_siteverify_api | string | `""` |  |
| clientapi.registration_disabled | bool | `true` | Prevents new users from being able to register on this homeserver, except when using the registration shared secret below. |
| clientapi.shared_secret | string | `""` | If set, allows registration by anyone who knows the shared secret, regardless of whether registration is otherwise disabled. |
| clientapi.turn.turn_password | string | `""` | The TURN password |
| clientapi.turn.turn_shared_secret | string | `""` |  |
| clientapi.turn.turn_uris | list | `[]` |  |
| clientapi.turn.turn_user_lifetime | string | `"24h"` | Duration for how long users should be considered valid ([see time.ParseDuration](https://pkg.go.dev/time#ParseDuration) for more) |
| clientapi.turn.turn_username | string | `""` | The TURN username |
| federationapi.disable_tls_validation | bool | `false` | Disable TLS validation |
| federationapi.prefer_direct_fetch | bool | `false` |  |
| federationapi.send_max_retries | int | `16` |  |
| global.cache.max_age | string | `"1h"` | The maximum amount of time that a cache entry can live for in memory before it will be evicted and/or refreshed from the database. Lower values result in easier admission of new cache entries but may also increase database load in comparison to higher values, so adjust conservatively. Higher values may make it harder for new items to make it into the cache, e.g. if new rooms suddenly become popular. |
| global.cache.max_size_estimated | string | `"1gb"` | The estimated maximum size for the global cache in bytes, or in terabytes, gigabytes, megabytes or kilobytes when the appropriate 'tb', 'gb', 'mb' or 'kb' suffix is specified. Note that this is not a hard limit, nor is it a memory limit for the entire process. A cache that is too small may ultimately provide little or no benefit. |
| global.database.conn_max_lifetime | int | `-1` | Default database maximum lifetime |
| global.database.host | string | `""` | Default database host |
| global.database.max_idle_conns | int | `2` | Default database maximum idle connections |
| global.database.max_open_conns | int | `90` | Default database maximum open connections |
| global.database.password | string | `""` | Default database password |
| global.database.user | string | `""` | Default database user |
| global.disable_federation | bool | `false` | Disable federation. Dendrite will not be able to make any outbound HTTP requests to other servers and the federation API will not be exposed. |
| global.dns_cache.cache_lifetime | string | `"10m"` | Duration for how long DNS cache items should be considered valid ([see time.ParseDuration](https://pkg.go.dev/time#ParseDuration) for more) |
| global.dns_cache.cache_size | int | `256` | Maximum number of entries to hold in the DNS cache |
| global.dns_cache.enabled | bool | `false` | Whether or not the DNS cache is enabled. |
| global.key_validity_period | string | `"168h0m0s"` |  |
| global.logging | list | [default dendrite config values](https://github.com/matrix-org/dendrite/blob/master/dendrite-config.yaml) | Default logging configuration |
| global.metrics.basic_auth.password | string | `"metrics"` | HTTP basic authentication password |
| global.metrics.basic_auth.user | string | `"metrics"` | HTTP basic authentication username |
| global.metrics.enabled | bool | `false` | Whether or not Prometheus metrics are enabled. |
| global.mscs | list | `["msc2946"]` | Configuration for experimental MSC's. (Valid values are: msc2836 and msc2946) |
| global.presence | object | `{"enable_inbound":false,"enable_outbound":false}` | Configures the handling of presence events. Inbound controls whether we receive presence events from other servers, outbound controls whether we send presence events for our local users to other servers. |
| global.profiling.enabled | bool | `false` | Enable pprof. You will need to manually create a port forwarding to the deployment to access PPROF, as it will only listen on localhost and the defined port. e.g. `kubectl port-forward deployments/dendrite 65432:65432` |
| global.profiling.port | int | `65432` | pprof port, if enabled |
| global.report_stats | object | `{"enabled":false,"endpoint":"https://matrix.org/report-usage-stats/push"}` | Configures phone-home statistics reporting. These statistics contain the server name, number of active users and some information on your deployment config. We use this information to understand how Dendrite is being used in the wild. |
| global.server_name | string | `""` | Servername for this Dendrite deployment |
| global.server_notices | object | `{"avatar_url":"","display_name":"Server Alerts","enabled":false,"local_part":"_server","room_name":"Server Alerts"}` | Server notices allows server admins to send messages to all users on the server. |
| global.tracing | object | disabled | Default tracing configuration |
| global.trusted_third_party_id_servers | list | `["matrix.org","vector.im"]` | Lists of domains that the server will trust as identity servers to verify third party identifiers such as phone numbers and email addresses. |
| global.well_known_client_name | string | `""` | The server name to delegate client-server communications to, with optional port e.g. localhost:443 |
| global.well_known_server_name | string | `""` | The server name to delegate server-server communications to, with optional port e.g. localhost:443 |
| image.name | string | `"ghcr.io/matrix-org/dendrite-monolith:v0.10.8"` | Docker repository/image to use |
| image.pullPolicy | string | `"IfNotPresent"` | Kubernetes pullPolicy |
| ingress.annotations | object | `{}` | Extra, custom annotations |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` | Create an ingress for a monolith deployment |
| ingress.hostName | string | `""` |  |
| ingress.hosts | list | `[]` |  |
| ingress.tls | list | `[]` |  |
| mediaapi.dynamic_thumbnails | bool | `false` |  |
| mediaapi.max_file_size_bytes | string | `"10485760"` | The max file size for uploaded media files |
| mediaapi.max_thumbnail_generators | int | `10` | The maximum number of simultaneous thumbnail generators to run. |
| mediaapi.thumbnail_sizes | list | [default dendrite config values](https://github.com/matrix-org/dendrite/blob/master/dendrite-config.yaml) | A list of thumbnail sizes to be generated for media content. |
| persistence.jetstream.capacity | string | `"1Gi"` |  |
| persistence.jetstream.existingClaim | string | `""` | Use an existing volume claim for jetstream |
| persistence.media.capacity | string | `"1Gi"` |  |
| persistence.media.existingClaim | string | `""` | Use an existing volume claim for media files |
| persistence.search.capacity | string | `"1Gi"` |  |
| persistence.search.existingClaim | string | `""` | Use an existing volume claim for the fulltext search index |
| persistence.storageClass | string | `""` |  |
| postgresql.auth.database | string | `"dendrite"` |  |
| postgresql.auth.password | string | `"changeme"` |  |
| postgresql.auth.username | string | `"dendrite"` |  |
| postgresql.enabled | bool | See value.yaml | Enable and configure postgres as the database for dendrite. |
| postgresql.image.repository | string | `"bitnami/postgresql"` |  |
| postgresql.image.tag | string | `"14.4.0"` |  |
| postgresql.persistence.enabled | bool | `false` |  |
| resources | object | sets some sane default values | Default resource requests/limits. |
| service.port | int | `80` |  |
| service.type | string | `"ClusterIP"` |  |
| signing_key.create | bool | `true` | Create a new signing key, if not exists |
| signing_key.existingSecret | string | `""` | Use an existing secret |
| syncapi.real_ip_header | string | `"X-Real-IP"` | This option controls which HTTP header to inspect to find the real remote IP address of the client. This is likely required if Dendrite is running behind a reverse proxy server. |
| syncapi.search | object | `{"enabled":false,"language":"en"}` | Configuration for the full-text search engine. |
| syncapi.search.enabled | bool | `false` | Whether or not search is enabled. |
| syncapi.search.language | string | `"en"` | The language most likely to be used on the server - used when indexing, to ensure the returned results match expectations. A full list of possible languages can be found [here](https://github.com/matrix-org/dendrite/blob/76db8e90defdfb9e61f6caea8a312c5d60bcc005/internal/fulltext/bleve.go#L25-L46) |
