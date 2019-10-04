FROM dendrite-build AS build
######################
## Client API Proxy ##
######################
FROM scratch AS client-api-proxy
EXPOSE 8008
ENV CLIENT_API_SERVER_URL="http://client-api:7771" SYNC_API_SERVER_URL="http://sync-api:7773" MEDIA_API_SERVER_URL="http://media-api:7774" PUBLIC_ROOMS_API_SERVER_URL="http://public-rooms-api:7775"
COPY --from=build /build/bin/client-api-proxy /bin/client-api-proxy
ENTRYPOINT ["/bin/client-api-proxy", "--bind-address=`:8008`"]
