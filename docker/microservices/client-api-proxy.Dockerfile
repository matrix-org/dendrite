FROM dendrite:latest as build-env

# Copy the binary from build container to final container
FROM alpine:3.10
COPY --from=build-env /app/bin/client-api-proxy /

# Component specific entrypoint, environment variables, and arguments
CMD /client-api-proxy                                           \
    -bind-address $CLIENT_BIND_ADDRESS                          \
    -client-api-server-url $CLIENT_API_SERVER_URL               \
    -sync-api-server-url $SYNC_API_SERVER_URL                   \
    -media-api-server-url $MEDIA_API_SERVER_URL                 \
    -public-rooms-api-server-url $PUBLIC_ROOMS_API_SERVER_URL
