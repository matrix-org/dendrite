# This dockerfile will build dendritejs and hook it up to riot-web, build that then dump the
# resulting HTML/JS onto an nginx container for hosting. It requires no specific build context
# as it pulls archives straight from github branches.
#
# $ docker build -t dendritejs -f DendriteJS.Dockerfile .
# $ docker run --rm -p 8888:80 dendritejs
# Then visit http://localhost:8888
FROM golang:1.14-alpine AS gobuild

# Download and build dendrite
WORKDIR /build
ADD https://github.com/matrix-org/dendrite/archive/main.tar.gz /build/main.tar.gz
RUN tar xvfz main.tar.gz
WORKDIR /build/dendrite-main
RUN GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/dendritejs


FROM node:14-stretch AS jsbuild
# apparently some deps require python
RUN apt-get update && apt-get -y install python

# Download riot-web and libp2p repos
WORKDIR /build
ADD https://github.com/matrix-org/go-http-js-libp2p/archive/main.tar.gz /build/libp2p.tar.gz
RUN tar xvfz libp2p.tar.gz
ADD https://github.com/vector-im/element-web/archive/matthew/p2p.tar.gz /build/p2p.tar.gz
RUN tar xvfz p2p.tar.gz

# Install deps for element-web, symlink in libp2p repo and build that too
WORKDIR /build/element-web-matthew-p2p
RUN yarn install
RUN ln -s /build/go-http-js-libp2p-master /build/element-web-matthew-p2p/node_modules/go-http-js-libp2p
RUN (cd node_modules/go-http-js-libp2p && yarn install)
COPY --from=gobuild /build/dendrite-main/main.wasm ./src/vector/dendrite.wasm
# build it all
RUN yarn build:p2p

SHELL ["/bin/bash", "-c"]
RUN echo $'\ 
    { \n\
    "default_server_config": { \n\
    "m.homeserver": { \n\
    "base_url": "https://p2p.riot.im", \n\
    "server_name": "p2p.riot.im" \n\
    }, \n\
    "m.identity_server": { \n\
    "base_url": "https://vector.im" \n\
    } \n\
    }, \n\
    "disable_custom_urls": false, \n\
    "disable_guests": true, \n\
    "disable_login_language_selector": false, \n\
    "disable_3pid_login": true, \n\
    "brand": "Riot", \n\
    "integrations_ui_url": "https://scalar.vector.im/", \n\
    "integrations_rest_url": "https://scalar.vector.im/api", \n\
    "integrations_widgets_urls": [ \n\
    "https://scalar.vector.im/_matrix/integrations/v1", \n\
    "https://scalar.vector.im/api", \n\
    "https://scalar-staging.vector.im/_matrix/integrations/v1", \n\
    "https://scalar-staging.vector.im/api", \n\
    "https://scalar-staging.riot.im/scalar/api" \n\
    ], \n\
    "integrations_jitsi_widget_url": "https://scalar.vector.im/api/widgets/jitsi.html", \n\
    "bug_report_endpoint_url": "https://riot.im/bugreports/submit", \n\
    "defaultCountryCode": "GB", \n\
    "showLabsSettings": false, \n\
    "features": { \n\
    "feature_pinning": "labs", \n\
    "feature_custom_status": "labs", \n\
    "feature_custom_tags": "labs", \n\
    "feature_state_counters": "labs" \n\
    }, \n\
    "default_federate": true, \n\
    "default_theme": "light", \n\
    "roomDirectory": { \n\
    "servers": [ \n\
    "matrix.org" \n\
    ] \n\
    }, \n\
    "welcomeUserId": "", \n\
    "piwik": { \n\
    "url": "https://piwik.riot.im/", \n\
    "whitelistedHSUrls": ["https://matrix.org"], \n\
    "whitelistedISUrls": ["https://vector.im", "https://matrix.org"], \n\
    "siteId": 1 \n\
    }, \n\
    "enable_presence_by_hs_url": { \n\
    "https://matrix.org": false, \n\
    "https://matrix-client.matrix.org": false \n\
    }, \n\
    "settingDefaults": { \n\
    "breadcrumbs": true \n\
    } \n\
    }' > webapp/config.json

FROM nginx
# Add "Service-Worker-Allowed: /" header so the worker can sniff traffic on this domain rather
# than just the path this gets hosted under. NB this newline echo syntax only works on bash.
SHELL ["/bin/bash", "-c"]
RUN echo $'\
    server { \n\
    listen 80; \n\
    add_header \'Service-Worker-Allowed\' \'/\'; \n\
    location / { \n\
    root   /usr/share/nginx/html; \n\
    index  index.html index.htm; \n\
    } \n\
    }' > /etc/nginx/conf.d/default.conf
RUN sed -i 's/}/    application\/wasm  wasm;\n}/g' /etc/nginx/mime.types
COPY --from=jsbuild /build/element-web-matthew-p2p/webapp /usr/share/nginx/html
