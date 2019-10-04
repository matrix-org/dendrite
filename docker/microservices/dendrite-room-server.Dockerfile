FROM dendrite:latest as build-env

# Copy the binary from build container to final container
FROM alpine:3.10
COPY --from=build-env /app/bin/dendrite-room-server /

# Component specific entrypoint, environment variables, and arguments
CMD /dendrite-room-server \
    --config $CONFIG