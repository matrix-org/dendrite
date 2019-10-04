FROM dendrite-build AS build
##############
## Sync API ##
##############
FROM scratch AS sync-api
EXPOSE 7773
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-sync-api-server /bin/sync-api
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/sync-api"]
