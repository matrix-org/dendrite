FROM dendrite:latest as build-env

# Copy the binary from build container to final container
FROM alpine:3.10
COPY --from=build-env /app/bin/dendrite-client-api-server /

# Component specific entrypoint, environment variables, and arguments
CMD /dendrite-client-api-server \
    --config $CONFIG