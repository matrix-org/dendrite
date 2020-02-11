# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs (see below) that
your PR should allow to pass to `sytest-whitelist` in dendrite's root
directory. Not all PRs need to make new tests pass. If we find your PR should
be making a test pass we may ask you to add to that file, as generally
Dendrite's progress can be tracked through the amount of SyTest tests it
passes.

## Finding out which tests to add

We recommend you run the tests locally by manually setting up SyTest or using a
SyTest docker image. After running the tests, a script will print the tests you
need to add to `sytest-whitelist`.

You should proceed after you see no build problems for dendrite after running:

```sh
./build.sh
```

### Manually Setting up SyTest

Make sure you have Perl v5+ installed, and get SyTest with:

(Note that this guide assumes your SyTest checkout is next to your
`dendrite` checkout.)

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
    user: dendrite
    database: dendrite
    host: 127.0.0.1
type: pg
EOF
```

Run the tests:

```sh
./run-tests.pl -I Dendrite::Monolith -d ../dendrite/bin -W ../dendrite/sytest-whitelist -O tap --all | tee results.tap
```

where `tee` lets you see the results while they're being piped to the file.

Once the tests are complete, run the helper script to see if you need to add
any newly passing test names to `sytest-whitelist` in the project's root directory:

```sh
../dendrite/show-expected-fail-tests.sh results.tap ../dendrite/sytest-whitelist ../dendrite/sytest-blacklist
```

If the script prints nothing/exits with 0, then you're good to go.

### Using a SyTest Docker image

Ensure you have the latest image for SyTest, then run the tests:

```sh
docker pull matrixdotorg/sytest-dendrite
docker run --rm -v /path/to/dendrite/:/src/ matrixdotorg/sytest-dendrite
```

where `/path/to/dendrite/` should be replaced with the actual path to your
dendrite source code. The output should tell you if you need to add any tests to
`sytest-whitelist`.
