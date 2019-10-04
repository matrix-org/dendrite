FROM dendrite-build AS build
######################
## Public Rooms API ##
######################
FROM scratch AS public-rooms-api
EXPOSE 7775
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-public-rooms-api-server /bin/public-rooms-api
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/public-rooms-api"]
