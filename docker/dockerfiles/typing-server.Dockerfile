FROM dendrite-build AS build
###################
## Typing Server ##
###################
FROM scratch AS typing-server
EXPOSE 7778
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-typing-server /bin/typing-server
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/typing-server"]
