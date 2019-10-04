FROM dendrite-build AS build
#######################
## Appservice Server ##
#######################
FROM scratch AS appservice-server
EXPOSE 7777
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-appservice-server /bin/appservice-server
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/appservice-server"]
