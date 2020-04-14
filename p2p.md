## Peer-to-peer Matrix

These are the instructions for setting up P2P Dendrite, current as of March 2020. There's both Go stuff and JS stuff to do to set this up.


### Dendrite

- The `master` branch has a WASM-only binary for dendrite: `./cmd/dendritejs`.
- Build it and copy assets to riot-web.
```
$ GOOS=js GOARCH=wasm go build -o main.wasm ./cmd/dendritejs
$ cp main.wasm ../riot-web/src/vector/dendrite.wasm
```

### Rendezvous

This is how peers discover each other and communicate.

By default, Dendrite uses the Matrix-hosted websocket star relay server at TODO `/dns4/ws-star.discovery.libp2p.io/tcp/443/wss/p2p-websocket-star`.
This is currently hard-coded in `./cmd/dendritejs/main.go` - you can also use a local one if you run your own relay:

```
$ npm install --global libp2p-websocket-star-rendezvous
$ rendezvous --port=9090 --host=127.0.0.1
```

Then use `/ip4/127.0.0.1/tcp/9090/ws/p2p-websocket-star/`.

### Riot-web

You need to check out these repos:

```
$ git clone git@github.com:matrix-org/go-http-js-libp2p.git
$ git clone git@github.com:matrix-org/go-sqlite3-js.git
```

Make sure to `yarn install` in both of these repos. Then:

- `$ cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" ./src/vector/`
- Comment out the lines in `wasm_exec.js` which contains:

```
if (!global.fs && global.require) {
    global.fs = require("fs");
}
```
- Add the diff at https://github.com/vector-im/riot-web/compare/matthew/p2p?expand=1 - ignore the `package.json` stuff.
- Add the following symlinks: they HAVE to be symlinks as the diff in `webpack.config.js` references specific paths.
```
$ cd node_modules
$ ln -s ../../go-sqlite-js # NB: NOT go-sqlite3-js
$ ln -s ../../go-http-js-libp2p
```

NB: If you don't run the server with `yarn start` you need to make sure your server is sending the header `Service-Worker-Allowed: /`.

TODO: Make a Docker image with all of this in it and a volume mount for `dendrite.wasm`.

### Running

You need a Chrome and a Firefox running to test locally as service workers don't work in incognito tabs.
- For Chrome, use `chrome://serviceworker-internals/` to unregister/see logs.
- For Firefox, use `about:debugging#/runtime/this-firefox` to unregister. Use the console window to see logs.

Assuming you've `yarn start`ed Riot-Web, go to `http://localhost:8080` and register with `http://localhost:8080` as your HS URL.

You can join rooms by room alias e.g `/join #foo:bar`.

### Known issues

- When registering you may be unable to find the server, it'll seem flakey. This happens because the SW, particularly in Firefox,
  gets killed after 30s of inactivity. When you are not registered, you aren't doing `/sync` calls to keep the SW alive, so if you
  don't register for a while and idle on the page, the HS will disappear. To fix, unregister the SW, and then refresh the page.

- The libp2p layer has rate limits, so frequent Federation traffic may cause the connection to drop and messages to not be transferred.
  I guess in other words, don't send too much traffic?

