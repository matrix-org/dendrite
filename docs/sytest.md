# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs (see below) that
your PR should allow to pass to `testfile` in dendrite's root directory. Not all
PRs need to make new tests pass. If we find your PR should be making a test pass
we may ask you to add to that file, as generally Dendrite's progress can be
tracked through the amount of SyTest tests it passes.

## Finding out which tests to add

We recommend you run the tests locally by manually setting up sytest or using a
sytest docker image. After running the tests, a script will print the tests you
need to add to `testfile` for you.

You should proceed after you see no build problems for dendrite by running:

```sh
./build.sh
```

### Manually Setting up sytest

Get a copy of sytest on the `develop` branch next to your dendrite
directory, and run the setup script:

```sh
git clone -b develop https://github.com/matrix-org/sytest
cd sytest
./install-deps.pl
```

Set up the database:

```sh
sudo -u postgres psql -c "CREATE USER dendrite PASSWORD 'itsasecret'"
sudo -u postgres psql -c "CREATE DATABASE sytest_template OWNER dendrite"
mkdir -p "server-0"
cat > "server-0/database.yaml" << EOF
args:
    user: $PGUSER
    database: $PGUSER
    host: $PGHOST
type: pg
EOF
```

Run the tests:

```sh
./run-tests.pl -I Dendrite::Monolith -d ../dendrite/bin -W ../dendrite/testfile -O tap --all | tee results.tap
```

where `tee` lets you see the results while they're being piped to the file.

Once the tests are complete, run the helper script to see if you need to add
anything, and add them as needed:

```sh
../dendrite/show-expected-fail-tests.sh results.tap
```

If the script prints nothing/exits with 0, then you're good to go.

### Using a sytest Docker Image

Ensure you have the latest image for sytest, then run the tests:

```sh
docker pull matrixdotorg/sytest-dendrite
docker run --rm -v /path/to/dendrite/:/src/ matrixdotorg/sytest-dendrite
```

where `/path/to/dendrite/` should be replaced with the actual path to your
dendrite source code. The output should tell you if you need to add any tests to
`testfile`.
