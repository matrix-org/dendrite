---
title: P2P Matrix
nav_exclude: true
---

# P2P Matrix

These are the instructions for setting up P2P Dendrite, current as of May 2020. There's both Go stuff and JS stuff to do to set this up.

## Dendrite

### Build

- The `main` branch has a WASM-only binary for dendrite: `./cmd/dendritejs`.
- Build it and copy assets to riot-web.

```
./build-dendritejs.sh
cp bin/main.wasm ../riot-web/src/vector/dendrite.wasm
```

### Test

To check that the Dendrite side is working well as Wasm, you can run the
Wasm-specific tests:

```
./test-dendritejs.sh
```

## Rendezvous

This is how peers discover each other and communicate.

By default, Dendrite uses the Matrix-hosted websocket star relay server at TODO `/dns4/ws-star.discovery.libp2p.io/tcp/443/wss/p2p-websocket-star`.
This is currently hard-coded in `./cmd/dendritejs/main.go` - you can also use a local one if you run your own relay:

```
npm install --global libp2p-websocket-star-rendezvous
rendezvous --port=9090 --host=127.0.0.1
```

Then use `/ip4/127.0.0.1/tcp/9090/ws/p2p-websocket-star/`.

## Riot-web

You need to check out this repo:

```
git clone git@github.com:matrix-org/go-http-js-libp2p.git
```

Make sure to `yarn install` in the repo. Then:

- `$ cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" ./src/vector/`
- Comment out the lines in `wasm_exec.js` which contains:

```
if (!global.fs && global.require) {
    global.fs = require("fs");
}
```

- Add the diff at <https://github.com/vector-im/riot-web/compare/matthew/p2p?expand=1> - ignore the `package.json` stuff.
- Add the following symlinks: they HAVE to be symlinks as the diff in `webpack.config.js` references specific paths.

```
cd node_modules
ln -s ../../go-http-js-libp2p
```

NB: If you don't run the server with `yarn start` you need to make sure your server is sending the header `Service-Worker-Allowed: /`.

TODO: Make a Docker image with all of this in it and a volume mount for `dendrite.wasm`.

## Running

You need a Chrome and a Firefox running to test locally as service workers don't work in incognito tabs.

- For Chrome, use `chrome://serviceworker-internals/` to unregister/see logs.
- For Firefox, use `about:debugging#/runtime/this-firefox` to unregister. Use the console window to see logs.

Assuming you've `yarn start`ed Riot-Web, go to `http://localhost:8080` and register with `http://localhost:8080` as your HS URL.

You can:

- join rooms by room alias e.g `/join #foo:bar`.
- invite specific users to a room.
- explore the published room list. All members of the room can re-publish aliases (unlike Synapse).
