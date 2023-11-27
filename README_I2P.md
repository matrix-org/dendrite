How to build a Dendrite Homeserver modified to run over I2P
===========================================================

1. First, clone the `matrix-org/dendrite` implementation of dendrite into your GOPATH and change directory to the `main` checkout.

```sh
git clone https://github.com/matrix-org/dendrite $HOME/go/src/github.com/matrix-org/dendrite
cd $HOME/go/src/github.com/matrix-org/dendrite
```

2. Second, add my fork `eyedeekay/dendrite` as a remote and check out the i2p-demo branch.

```sh
git remote add idk https://github.com/eyedeekay/dendrite
git pull idk i2p-demo
git checkout i2p-demo
```

3. Third, build the binary:

```sh
go build -o bin/dendrite-demo-i2p ./cmd/dendrite-demo-i2p
```

Or, do it automatically using just a `curlpipe`
-----------------------------------------------

```sh
curl 'https://eyedeekay.github.io/dendrite/update-branch.sh' | bash -
```