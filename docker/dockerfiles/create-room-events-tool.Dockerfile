FROM dendrite-build AS build
#############################
## Create Room Events Tool ##
#############################
FROM scratch AS create-room-events-tool
COPY --from=build /build/bin/create-room-events /bin/create-room-events
ENTRYPOINT ["/bin/create-room-events"]
