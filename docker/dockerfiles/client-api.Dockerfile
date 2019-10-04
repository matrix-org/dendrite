FROM dendrite-build AS build
################
## Client API ##
################
FROM scratch AS client-api
EXPOSE 7771
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-client-api-server /bin/client-api
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/client-api"]
