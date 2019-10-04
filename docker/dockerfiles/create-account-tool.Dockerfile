FROM dendrite-build AS build
#########################
## Create Account Tool ##
#########################
FROM scratch AS create-account-tool
COPY --from=build /build/bin/create-account /bin/create-account
ENTRYPOINT ["/bin/create-account"]
