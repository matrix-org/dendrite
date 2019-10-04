FROM dendrite-build AS build
####################
## Federation API ##
####################
FROM scratch AS federation-api
EXPOSE 7772
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-federation-api-server /bin/federation-api
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/federation-api"]
