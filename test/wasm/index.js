#!/usr/bin/env node

/*
Copyright 2021 The Matrix.org Foundation C.I.C.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

const fs = require('fs');
const path = require('path');
const childProcess = require('child_process');

(async function() {
    // sql.js
    const initSqlJs = require('sql.js');
    await initSqlJs().then(SQL => {
        global._go_sqlite = SQL;
        console.log("Loaded sqlite")
    });
    // dendritejs expects to write to `/idb` so we create that here
    // Since this is testing only, we use the default in-memory FS
    global._go_sqlite.FS.mkdir("/idb");

    // WebSocket
    const WebSocket = require('isomorphic-ws');
    global.WebSocket = WebSocket;

    // Load the generic Go Wasm exec helper inline to trigger built-in run call
    // This approach avoids copying `wasm_exec.js` into the repo, which is nice
    // to aim for since it can differ between Go versions.
    const goRoot = await new Promise((resolve, reject) => {
        childProcess.execFile('go', ['env', 'GOROOT'], (err, out) => {
            if (err) {
                reject("Can't find go");
            }
            resolve(out.trim());
        });
    });
    const execPath = path.join(goRoot, 'misc/wasm/wasm_exec.js');
    const execCode = fs.readFileSync(execPath, 'utf8');
    eval(execCode);
})();
