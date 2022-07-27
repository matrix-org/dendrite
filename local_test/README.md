# Run local dendirete test server

Build from project root:

```bash
./build.sh
```

Modify `vars.env` and `dendrite.yaml` as desired and deploy:

```bash
./local_test/deploys.sh
```

Run all dendrites in background:

```bash
./local_test/run_all.sh
```

Stop all dendrites runnign in background:

```bash
./local_test/stop_all.sh
```

Run single dendrite number N in foreground:

```bash
./local_test/run_single.sh 0
```

Delete deployment:

```bash
./local_test/clean.sh 0
```
