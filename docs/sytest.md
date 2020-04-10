# SyTest

Dendrite uses [SyTest](https://github.com/matrix-org/sytest) for its
integration testing. When creating a new PR, add the test IDs (see below) that
your PR should allow to pass to `sytest-whitelist` in dendrite's root
directory. Not all PRs need to make new tests pass. If we find your PR should
be making a test pass we may ask you to add to that file, as generally
Dendrite's progress can be tracked through the amount of SyTest tests it
passes.

## Finding out which tests to add

We recommend you run the tests locally by using the SyTest docker image or
manually setting up SyTest. After running the tests, a script will print the
tests you need to add to `sytest-whitelist`.

You should proceed after you see no build problems for dendrite after running:

```sh
./build.sh
```

### Using a SyTest Docker image

Ensure you have the latest image for SyTest, then run the tests:

```sh
docker pull matrixdotorg/sytest-dendrite
docker run --rm -v /path/to/dendrite/:/src/ matrixdotorg/sytest-dendrite
```

where `/path/to/dendrite/` should be replaced with the actual path to your
dendrite source code. The output should tell you if you need to add any tests to
`sytest-whitelist`.

### Manually Setting up SyTest

If you don't want to use the Docker image, you can also run SyTest by hand. Make
sure you have Perl 5 or above, and get SyTest with:

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
for i in dendrite0 dendrite1 sytest_template; do sudo -u postgres psql -c "CREATE DATABASE $i OWNER dendrite;"; done
mkdir -p "server-0"
cat > "server-0/database.yaml" << EOF
args:
    user: dendrite
    password: itsasecret
    database: dendrite0
    host: 127.0.0.1
    sslmode: disable
type: pg
EOF
mkdir -p "server-1"
cat > "server-1/database.yaml" << EOF
args:
    user: dendrite
    password: itsasecret
    database: dendrite1
    host: 127.0.0.1
    sslmode: disable
type: pg
EOF
```

Run the tests:

```sh
POSTGRES=1 ./run-tests.pl -I Dendrite::Monolith -d ../dendrite/bin -W ../dendrite/sytest-whitelist -O tap --all | tee results.tap
```

where `tee` lets you see the results while they're being piped to the file, and
`POSTGRES=1` enables testing with PostgeSQL. If the `POSTGRES` environment
variable is not set or is set to 0, SyTest will fall back to SQLite 3.

Once the tests are complete, run the helper script to see if you need to add
any newly passing test names to `sytest-whitelist` in the project's root directory:

```sh
../dendrite/show-expected-fail-tests.sh results.tap ../dendrite/sytest-whitelist ../dendrite/sytest-blacklist
```

If the script prints nothing/exits with 0, then you're good to go.
