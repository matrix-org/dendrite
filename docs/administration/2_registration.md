---
title: Enabling registration
parent: Administration
permalink: /administration/registration
nav_order: 2
---

# Enabling registration

Enabling registration allows users to register their own user accounts on your
Dendrite server using their Matrix client. They will be able to choose their own
username and password and log in.

Registration is controlled by the `registration_disabled` field in the `client_api`
section of the configuration. By default, `registration_disabled` is set to `true`,
disabling registration. If you want to enable registration, you should change this
setting to `false`.

Currently Dendrite supports secondary verification using [reCAPTCHA](https://www.google.com/recaptcha/about/).
Other methods will be supported in the future.

## reCAPTCHA verification

Dendrite supports reCAPTCHA as a secondary verification method. If you want to enable
registration, it is **highly recommended** to configure reCAPTCHA. This will make it
much more difficult for automated spam systems from registering accounts on your
homeserver automatically.

You will need an API key from the [reCAPTCHA Admin Panel](https://www.google.com/recaptcha/admin).
Then configure the relevant details in the `client_api` section of the configuration:

```yaml
client_api:
  # ...
  registration_disabled: false
  recaptcha_public_key: "PUBLIC_KEY_HERE"
  recaptcha_private_key: "PRIVATE_KEY_HERE"
  enable_registration_captcha: true
  captcha_bypass_secret: ""
  recaptcha_siteverify_api: "https://www.google.com/recaptcha/api/siteverify"
```

## Open registration

Dendrite does support open registration — that is, allowing users to create their own
user accounts without any verification or secondary authentication. However, it
is **not recommended** to enable open registration, as this leaves your homeserver
vulnerable to abuse by spammers or attackers, who create large numbers of user
accounts on Matrix homeservers in order to send spam or abuse into the network.

It isn't possible to enable open registration in Dendrite in a single step. If you
try to disable the `registration_disabled` option without any secondary verification
methods enabled (such as reCAPTCHA), Dendrite will log an error and fail to start.
