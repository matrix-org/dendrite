FROM dendrite-build AS build
##########################
## Federation API Proxy ##
##########################
FROM scratch AS federation-api-proxy
EXPOSE 8008
ENV FEDERATION_API_SERVER_URL="http://federation-api:7772" MEDIA_API_SERVER_URL="http://media-api:7774"
COPY --from=build /build/bin/federation-api-proxy /bin/federation-api-proxy
ENTRYPOINT ["/bin/federation-api-proxy", "--bind-address=`:8008`"]
