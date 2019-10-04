FROM dendrite-build AS build
###############
## Media API ##
###############
FROM scratch AS media-api
EXPOSE 7774
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-media-api-server /bin/media-api
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/media-api"]
