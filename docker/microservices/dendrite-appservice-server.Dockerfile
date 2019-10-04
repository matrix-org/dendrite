FROM dendrite:latest as build-env

# Copy the binary from build container to final container
FROM alpine:3.10
COPY --from=build-env /app/bin/dendrite-appservice-server /

# Component specific entrypoint, environment variables, and arguments
CMD /dendrite-appservice-server \
    --config $CONFIG