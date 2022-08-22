# dendrite

![Version: 7.1.1](https://img.shields.io/badge/Version-7.1.1-informational?style=flat-square) ![AppVersion: v0.9.4](https://img.shields.io/badge/AppVersion-v0.9.4-informational?style=flat-square)

Dendrite Matrix Homeserver

**This chart is not maintained by the upstream project and any issues with the chart should be raised [here](https://github.com/samipsolutions/helm-charts/issues/new/choose)**

## Source Code

* <https://github.com/matrix-org/dendrite>
* <https://github.com/matrix-org/dendrite/tree/master/build/docker>

## Requirements

Kubernetes: `>=1.19.0-0`

## Dependencies

| Repository | Name | Version |
|------------|------|---------|
| https://bjw-s.github.io/helm-charts/ | common | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | keyserver(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | clientapi(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | mediaapi(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | syncapi(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | roomserver(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | federationapi(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | userapi(common) | 0.1.0 |
| https://bjw-s.github.io/helm-charts/ | appserviceapi(common) | 0.1.0 |
| https://nats-io.github.io/k8s/helm/charts/ | nats | 0.17.1 |

## TL;DR

```console
helm repo add samipsolutions https://helm.samipsolutions.fi/
helm repo update
helm install dendrite samipsolutions/dendrite
```

## Installing the Chart

To install the chart with the release name `dendrite`

```console
helm install dendrite samipsolutions/dendrite
```

## Uninstalling the Chart

To uninstall the `dendrite` deployment

```console
helm uninstall dendrite
```

The command removes all the Kubernetes components associated with the chart **including persistent volumes** and deletes the release.

## Configuration

Read through the [values.yaml](./values.yaml) file. It has several commented out suggested values.
Other values may be used from the [values.yaml](https://github.com/k8s-at-home/library-charts/tree/main/charts/stable/common/values.yaml) from the [common library](https://github.com/k8s-at-home/library-charts/tree/main/charts/stable/common).

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

```console
helm install dendrite \
  --set env.TZ="America/New York" \
    samipsolutions/dendrite
```

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart.

```console
helm install dendrite samipsolutions/dendrite -f values.yaml
```

## Custom configuration

### Polylith Ingress

Due to the complexity of setting up ingress for each individual component it
is left up to the individual to add the necessary ingress fields to polylith deployments.

For more information see:
- https://github.com/matrix-org/dendrite/blob/master/docs/INSTALL.md#nginx-or-other-reverse-proxy
- and https://github.com/matrix-org/dendrite/blob/master/docs/nginx/polylith-sample.conf

## Values

**Important**: When deploying an application Helm chart you can add more values from our common library chart [here](https://github.com/k8s-at-home/library-charts/tree/main/charts/stable/common)

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| appserviceapi | object | See values.yaml | Configure the app service api. For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| appserviceapi.database | object | See values.yaml | Override general dendrite.database parameters. |
| appserviceapi.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| appserviceapi.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| appserviceapi.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| appserviceapi.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| appserviceapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| appserviceapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| appserviceapi.image.tag | string | chart.appVersion | image tag |
| clientapi | object | See values.yaml | Configuration for the client api component. For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| clientapi.config.captcha | object | See values.yaml | Configure captcha for registration |
| clientapi.config.rate_limiting | object | values.yaml | Configure rate limiting. |
| clientapi.config.registration_disabled | bool | `true` | Enable or disable registration for this homeserver. |
| clientapi.config.registration_shared_secret | string | `""` | Shared secret that allows registration, despite registration_disabled. |
| clientapi.config.turn | object | See values.yaml | Configure TURN |
| clientapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| clientapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| clientapi.image.tag | string | chart.appVersion | image tag |
| database.conn_max_lifetime | int | `-1` |  |
| database.connection_string | string | `"file:dendrite?sslmode=disable"` |  |
| database.max_idle_conns | int | `2` |  |
| database.max_open_conns | int | `100` |  |
| dendrite | object | See values.yaml | Configuration for Dendrite. For more information see [the sample denrite-config.yaml](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| dendrite.global | object | See values.yaml | Configure the global settings for dendrite. |
| dendrite.global.cache | object | `{"max_age":"1h","max_size_estimated":"1gb"}` | Congigure the in-memory caches |
| dendrite.global.cache.max_age | string | `"1h"` | The maximum amount of time that a cache entry can live for in memory |
| dendrite.global.cache.max_size_estimated | string | `"1gb"` | Configure the maximum estimated cache size (not a hard limit) |
| dendrite.global.disable_federation | bool | `false` | Disables federation |
| dendrite.global.dns_cache | object | See values.yaml | Configure DNS cache. |
| dendrite.global.dns_cache.enabled | bool | See values.yaml | If enabled, dns cache will be enabled. |
| dendrite.global.key_validity_period | string | `"168h0m0s"` | Configure the key_validity period |
| dendrite.global.metrics | object | See values.yaml | Configure prometheus metrics collection for dendrite. |
| dendrite.global.metrics.enabled | bool | See values.yaml | If enabled, metrics collection will be enabled |
| dendrite.global.mscs | list | `[]` | Configure experimental MSC's |
| dendrite.global.presence | object | `{"enable_inbound":false,"enable_outbound":false}` | Configure handling of presence events |
| dendrite.global.presence.enable_inbound | bool | `false` | Whether inbound presence events are allowed, e.g. receiving presence events from other servers |
| dendrite.global.presence.enable_outbound | bool | `false` | Whether outbound presence events are allowed, e.g. sending presence events to other servers |
| dendrite.global.server_name | string | `"localhost"` | (required) Configure the server name for the dendrite instance. |
| dendrite.global.server_notices | object | `{"avatar_url":"","display_name":"Server alerts","enabled":false,"local_part":"_server","room_name":"Server Alerts"}` | Server notices allows server admins to send messages to all users. |
| dendrite.global.server_notices.avatar_url | string | `""` | The mxid of the avatar to use |
| dendrite.global.server_notices.display_name | string | `"Server alerts"` | The displayname to be used when sending notices |
| dendrite.global.server_notices.local_part | string | `"_server"` | The server localpart to be used when sending notices, ensure this is not yet taken |
| dendrite.global.server_notices.room_name | string | `"Server Alerts"` | The roomname to be used when creating messages |
| dendrite.global.trusted_third_party_id_servers | list | `["matrix.org","vector.im"]` | Configure the list of domains the server will trust as identity servers |
| dendrite.global.well_known_client_name | string | `""` | Configure the well-known client name and optional port |
| dendrite.global.well_known_server_name | string | `""` | Configure the well-known server name and optional port |
| dendrite.logging | list | See values.yaml | Configure logging. |
| dendrite.matrix_key_secret.create | bool | `false` | Create matrix_key secret using the keyBody below. |
| dendrite.matrix_key_secret.existingSecret | string | `""` | Use an existing secret |
| dendrite.matrix_key_secret.keyBody | string | `""` | New Key Body |
| dendrite.matrix_key_secret.secretPath | string | `"matrix_key.pem"` | Field in the secret to get the key from |
| dendrite.polylithEnabled | bool | `false` | Enable polylith deployment |
| dendrite.polylith_ingress | object | See values.yaml | Enable and configure polylith ingress as per https://github.com/matrix-org/dendrite/blob/main/docs/nginx/polylith-sample.conf |
| dendrite.polylith_ingress.syncapi_paths | list | See values.yaml | Sync API Paths are a little tricky since they require regular expressions. Therefore the paths will depend on the ingress controller used. See values.yaml for nginx and traefik. |
| dendrite.report_stats | object | `{"enabled":false,"endpoint":""}` | Usage statistics reporting configuration |
| dendrite.report_stats.enabled | bool | false | Enable or disable usage reporting |
| dendrite.report_stats.endpoint | string | `""` | Push endpoint for usage statistics |
| dendrite.tls_secret | object | See values.yaml | If enabled, use an existing secrets for the TLS certificate and key. Otherwise, to enable TLS a `server.crt` and `server.key` must be mounted at `/etc/dendrite`. |
| dendrite.tracing | object | See values.yaml | Configure opentracing. |
| federationapi | object | values.yaml | Configure the Federation API For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| federationapi.database | object | See values.yaml | Override general dendrite.database parameters. |
| federationapi.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| federationapi.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| federationapi.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| federationapi.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| federationapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| federationapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| federationapi.image.tag | string | chart.appVersion | image tag |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/matrix-org/dendrite-monolith","tag":null}` |  IMPORTANT NOTE This chart inherits from our common library chart. You can check the default values/options here: https://github.com/k8s-at-home/library-charts/tree/main/charts/stable/common/values.yaml |
| image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| image.repository | string | `"ghcr.io/matrix-org/dendrite-monolith"` | image repository |
| image.tag | string | chart.appVersion | image tag |
| ingress.main | object | See values.yaml | (Monolith Only) Enable and configure ingress settings for the chart under this key. |
| keyserver | object | See values.yaml | Configure the key server. For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| keyserver.database | object | See values.yaml | Override general dendrite.database parameters. |
| keyserver.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| keyserver.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| keyserver.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| keyserver.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| keyserver.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| keyserver.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| keyserver.image.tag | string | chart.appVersion | image tag |
| mediaapi | object | values.yaml | Configure the Media API For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| mediaapi.database | object | See values.yaml | Override general dendrite.database parameters. |
| mediaapi.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| mediaapi.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| mediaapi.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| mediaapi.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| mediaapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| mediaapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| mediaapi.image.tag | string | chart.appVersion | image tag |
| mscs | object | values.yaml | Configuration for experimental MSCs For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| mscs.database | object | See values.yaml | Override general dendrite.database parameters. |
| mscs.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| mscs.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| mscs.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| mscs.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| nats.enabled | bool | See value.yaml | Enable and configure NATS for dendrite. Can be disabled for monolith deployments - an internal NATS server will be used in its place. |
| nats.nats.image | string | `"nats:2.7.1-alpine"` |  |
| nats.nats.jetstream.enabled | bool | `true` |  |
| persistence | object | See values.yaml | Configure persistence settings for the chart under this key. |
| persistence.jetstream | object | See values.yaml | Configure Jetsream persistence. This is highly recommended in production. |
| roomserver | object | values.yaml | Configure the Room Server For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| roomserver.database | object | See values.yaml | Override general dendrite.database parameters. |
| roomserver.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| roomserver.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| roomserver.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| roomserver.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| roomserver.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| roomserver.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| roomserver.image.tag | string | chart.appVersion | image tag |
| service | object | See values.yaml | If added dendrite will start a HTTP and HTTPS listener args:   - "--tls-cert=server.crt"   - "--tls-key=server.key" -- Configures service settings for the chart. |
| service.main.ports.http | object | See values.yaml | Configures the default HTTP listener for dendrite |
| service.main.ports.https | object | See values.yaml | Configures the HTTPS listener for dendrite |
| syncapi | object | values.yaml | Configure the Sync API For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| syncapi.database | object | See values.yaml | Override general dendrite.database parameters. |
| syncapi.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| syncapi.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| syncapi.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| syncapi.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| syncapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| syncapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| syncapi.image.tag | string | chart.appVersion | image tag |
| userapi | object | values.yaml | Configure the User API For more information see [the sample dendrite configuration](https://github.com/matrix-org/dendrite/blob/main/dendrite-sample.polylith.yaml) |
| userapi.config.bcrypt_cost | int | 10 | bcrypt cost (2^[cost] = rounds) |
| userapi.database | object | See values.yaml | Override general dendrite.database parameters. |
| userapi.database.conn_max_lifetime | string | dendrite.database.conn_max_lifetime | Maximum connection lifetime |
| userapi.database.connection_string | string | file or derived from included postgresql deployment | Custom connection string |
| userapi.database.max_idle_conns | string | dendrite.database.max_idle_conns | Maximum dile connections |
| userapi.database.max_open_conns | string | dendrite.database.max_open_conns | Maximum open connections |
| userapi.image.pullPolicy | string | `"IfNotPresent"` | image pull policy |
| userapi.image.repository | string | `"matrixdotorg/dendrite-polylith"` | image repository |
| userapi.image.tag | string | chart.appVersion | image tag |

## Changelog

### Version 7.1.1

#### Added

N/A

#### Changed

N/A

#### Fixed

* Global database config

### Older versions

A historical overview of changes can be found on [ArtifactHUB](https://artifacthub.io/packages/helm/samipsolutions/dendrite?modal=changelog)

## Support

- See the [Docs](https://docs.k8s-at-home.com/our-helm-charts/getting-started/)
- Open an [issue](https://github.com/samipsolutions/helm-charts/issues/new/choose)
- Ask a [question](https://github.com/k8s-at-home/organization/discussions)
- Join our [Discord](https://discord.gg/sTMX7Vh) community

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v0.1.1](https://github.com/k8s-at-home/helm-docs/releases/v0.1.1)
