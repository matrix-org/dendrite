# Application Service

This component interfaces with external [Application
Services](https://matrix.org/docs/spec/application_service/unstable.html).
This includes any HTTP endpoints that Application Services call, as well as talking
to any HTTP endpoints that Application Services provide themselves.

## Consumers

This component consumes and filters events from the Roomserver Kafka stream, passing on any necessary events to subscribing Application Services.