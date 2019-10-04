FROM dendrite-build AS build
########################
## Generate Keys Tool ##
########################
FROM scratch AS generate-keys-tool
COPY --from=build /build/bin/generate-keys /bin/generate-keys
ENTRYPOINT ["/bin/generate-keys"]
