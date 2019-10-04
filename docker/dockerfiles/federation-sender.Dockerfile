FROM dendrite-build AS build
#######################
## Federation Sender ##
#######################
FROM scratch AS federation-sender
EXPOSE 7776
ENV DENDRITE_CONFIG_FILE="dendrite.yaml"
WORKDIR /dendrite
COPY --from=build /build/bin/dendrite-federation-sender-server /bin/federation-sender
COPY --from=build /build/docker/dendrite-docker.yml ./dendrite.yaml
ENTRYPOINT ["/bin/federation-sender"]
