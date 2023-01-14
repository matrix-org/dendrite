# Dendrite

# Fork!
## Differences to upstream:
- Replace official Pinecone with my fork of it that contains a [patch to fix an issue](https://github.com/BieHDC/pinecone/commit/87da81cee2e39b2f109e4a9ec156c526ae4a5616) with custom routers.
- Added `cmd/dendrite-demo-pinecone-i2p`


## How to run
See the [Readme](https://github.com/BieHDC/dendrite/tree/main/cmd/dendrite-demo-pinecone-i2p/README.md).


## How to build
```
cd cmd/dendrite-demo-pinecone-i2p
go build
```
Needs Go 1.18 or later. Your distro should take care of it.


## Todo
- Do real world testing.
- Check for privacy leaks.
- Figure out how to forward the :8008 client listener and make clients connect over .i2p aswell (for non self hosters).
- Upstream the pinecone-"fix"?
- Fix the DialContext [issue of the SAM lib](https://github.com/eyedeekay/sam3/blob/45106d2b7062a690dfad30841163b510855469df/stream.go#L117)
- Monitor [store and forward pr](https://github.com/matrix-org/dendrite/pull/2917). We need to sync it with i2p.
- Monitor cross-network stuff, generally [this](https://arewep2pyet.com/).
- ...


For more information about Dendrite itself check out the [main repo](https://github.com/matrix-org/dendrite).
Do not report bugs to them that might be our bugs.
