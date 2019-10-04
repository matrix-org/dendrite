FROM dendrite-build AS build
####################
## Kafka Producer ##
####################
FROM scratch AS kafka-producer
COPY --from=build /build/bin/kafka-producer /bin/kafka-producer
ENTRYPOINT ["/bin/kafka-producer"]
