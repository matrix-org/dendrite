FROM dendrite-build AS build
#################
## Room Server ##
#################
FROM scratch AS room-server
EXPOSE 7770
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-room-server /bin/room-server
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/room-server"]
