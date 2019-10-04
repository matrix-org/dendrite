FROM dendrite:latest as build-env

# Copy the binary from build container to final container
FROM alpine:3.10
COPY --from=build-env /app/bin/federation-api-proxy /

# Component specific entrypoint, environment variables, and arguments
CMD /federation-api-proxy                       \
    -bind-address $FEDERATION_BIND_ADDRESS      \
    -media-api-server-url $MEDIA_API_SERVER_URL \
    -federation-api-url $FEDERATION_API_URL     \
    -tls-cert $FEDERATION_TLS_CERT              \
    -tls-key $FEDERATION_TLS_KEY                