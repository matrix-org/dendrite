## Key Server

This is an internal component which manages E2E keys from clients. It handles all the [Key Management APIs](https://matrix.org/docs/spec/client_server/r0.6.1#key-management-api) with the exception of `/keys/changes` which is handled by Sync API. This component is designed to shard by user ID.

Keys are uploaded and stored in this component, and key changes are emitted to a Kafka topic for downstream components such as Sync API.

### Internal APIs
- `PerformUploadKeys` stores identity keys and one-time public keys for given user(s).
- `PerformClaimKeys` acquires one-time public keys for given user(s). This may involve outbound federation calls.
- `QueryKeys` returns identity keys for given user(s). This may involve outbound federation calls. This component may then cache federated identity keys to avoid repeatedly hitting remote servers.
- A topic which emits identity keys every time there is a change (addition or deletion).

### Endpoint mappings
- Client API maps `/keys/upload` to `PerformUploadKeys`.
- Client API maps `/keys/query` to `QueryKeys`.
- Client API maps `/keys/claim` to `PerformClaimKeys`.
- Federation API maps `/user/keys/query` to `QueryKeys`.
- Federation API maps `/user/keys/claim` to `PerformClaimKeys`.
- Sync API maps `/keys/changes` to consuming from the Kafka topic.
