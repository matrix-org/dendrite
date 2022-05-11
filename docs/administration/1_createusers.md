---
title: Creating user accounts
parent: Administration
permalink: /administration/createusers
nav_order: 1
---

# Creating user accounts

User accounts can be created on a Dendrite instance in a number of ways.

## From the command line

The `create-account` tool is built in the `bin` folder when building Dendrite with
the `build.sh` script.

It uses the `dendrite.yaml` configuration file to connect to the Dendrite user database
and create the account entries directly. It can therefore be used even if Dendrite is not
running yet, as long as the database is up.

An example of using `create-account` to create a **normal account**:

```bash
./bin/create-account -config /path/to/dendrite.yaml -username USERNAME
```

You will be prompted to enter a new password for the new account.

To create a new **admin account**, add the `-admin` flag:

```bash
./bin/create-account -config /path/to/dendrite.yaml -username USERNAME -admin
```

## Using shared secret registration

Dendrite supports the Synapse-compatible shared secret registration endpoint.

To enable shared secret registration, you must first enable it in the `dendrite.yaml`
configuration file by specifying a shared secret. In the `client_api` section of the config,
enter a new secret into the `registration_shared_secret` field:

```yaml
client_api:
  # ...
  registration_shared_secret: ""
```

You can then use the `/_synapse/admin/v1/register` endpoint as per the
[Synapse documentation](https://matrix-org.github.io/synapse/latest/admin_api/register_api.html).

Shared secret registration is only enabled once a secret is configured. To disable shared
secret registration again, remove the secret from the configuration file.
