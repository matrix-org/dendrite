# Server Key Format

Dendrite stores the server signing key in the PEM format with the following structure.

```
-----BEGIN MATRIX PRIVATE KEY-----
Key-ID: ed25519:<Key Handle>

<Base64 Encoded Key Data>
-----END MATRIX PRIVATE KEY-----
```

## Converting Synapse Keys

If you have signing keys from a previous synapse server, you should ideally configure them as `old_private_keys` in your Dendrite config file. Synapse stores signing keys in the following format.

```
ed25519 <Key Handle> <Base64 Encoded Key Data>
```

To convert this key to Dendrite's PEM format, use the following template. **It is important to include the equals sign, as the key data needs to be padded to 32 bytes.**

```
-----BEGIN MATRIX PRIVATE KEY-----
Key-ID: ed25519:<Key Handle>

<Base64 Encoded Key Data>=
-----END MATRIX PRIVATE KEY-----
```