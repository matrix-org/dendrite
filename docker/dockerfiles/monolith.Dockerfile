FROM dendrite-build AS build

##############
## Monolith ##
##############
FROM scratch AS monolith
EXPOSE 8008 8448
ENV DENDRITE_CONFIG_FILE="dendrite.yaml" TLS_CERT_FILE="server.crt" TLS_KEY_FILE="server.key"
WORKDIR /dendrite
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /build/bin/dendrite-monolith-server /bin/monolith
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/monolith"]
