---
title: Generating signing keys
parent: Installation
nav_order: 8
permalink: /installation/signingkeys
---

# Generating signing keys

All Matrix homeservers require a signing private key, which will be used to authenticate
federation requests and events.

The `generate-keys` utility can be used to generate a private key. Assuming that Dendrite was
built using `build.sh`, you should find the `generate-keys` utility in the `bin` folder.

To generate a Matrix signing private key:

```bash
./bin/generate-keys --private-key matrix_key.pem
```

The generated `matrix_key.pem` file is your new signing key.

## Important warning

You must treat this key as if it is highly sensitive and private, so **never share it with
anyone**. No one should ever ask you for this key for any reason, even to debug a problematic
Dendrite server.

Make sure take a safe backup of this key. You will likely need it if you want to reinstall
Dendrite, or any other Matrix homeserver, on the same domain name in the future. If you lose
this key, you may have trouble joining federated rooms.

## Old signing keys

If you already have old signing keys from a previous Matrix installation on the same domain
name, you can reuse those instead, as long as they have not been previously marked as expired —
a key that has been marked as expired in the past is unusable.

Old keys from a previous Dendrite installation can be reused as-is without any further
configuration required. Simply use that key file in the Dendrite configuration.

If you have server keys from an older Synapse instance, you can convert them to Dendrite's PEM
format and configure them as `old_private_keys` in your config.

## Key format

Dendrite stores the server signing key in the PEM format with the following structure.

```
-----BEGIN MATRIX PRIVATE KEY-----
Key-ID: ed25519:<Key ID>

<Base64 Encoded Key Data>
-----END MATRIX PRIVATE KEY-----
```

## Converting Synapse keys

If you have signing keys from a previous Synapse installation, you should ideally configure them
as `old_private_keys` in your Dendrite config file. Synapse stores signing keys in the following
format:

```
ed25519 <Key ID> <Base64 Encoded Key Data>
```

To convert this key to Dendrite's PEM format, use the following template. You must copy the Key ID
exactly without modifying it. **It is important to include the trailing equals sign on the Base64
Encoded Key Data** if it is not already present in the original key, as the key data needs to be
padded to exactly 32 bytes:

```
-----BEGIN MATRIX PRIVATE KEY-----
Key-ID: ed25519:<Key ID>

<Base64 Encoded Key Data>=
-----END MATRIX PRIVATE KEY-----
```
