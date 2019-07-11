This component roughly corresponds to "Client Room Send" and "Client Sync" on [the WIRING diagram](https://github.com/matrix-org/dendrite/blob/master/WIRING.md).
This component produces multiple binaries.

## Internals

- HTTP routing is done using `gorilla/mux` and the routing paths are in the `routing` package.

### Writers
- Each HTTP "write operation" (`/createRoom`, `/rooms/$room_id/send/$type`, etc) is contained entirely to a single file in the `writers` package.
- This file contains the request and response `struct` definitions, as well as a `Validate() bool` function to validate incoming requests.
- The entry point for each write operation is a stand-alone function as this makes testing easier. All dependencies should be injected into this function, including server keys/name, etc.
