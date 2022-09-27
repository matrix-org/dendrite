---
title: Enabling presence
parent: Administration
permalink: /administration/presence
nav_order: 3
---

# Enabling presence

Dendrite supports presence, which allows you to send your online/offline status
to other users, and to receive their statuses automatically. They will be displayed
by supported clients.

Note that enabling presence **can negatively impact** the performance of your Dendrite
server — it will require more CPU time and will increase the "chattiness" of your server
over federation. It is disabled by default for this reason.

Dendrite has two options for controlling presence:

* **Enable inbound presence**: Dendrite will handle presence updates for remote users
  and distribute them to local users on your homeserver;
* **Enable outbound presence**: Dendrite will generate presence notifications for your
  local users and distribute them to remote users over the federation.

This means that you can configure only one or other direction if you prefer, i.e. to
receive presence from other servers without revealing the presence of your own users.

## Configuring presence

Presence is controlled by the `presence` block in the `global` section of the
configuration file:

```yaml
global:
  # ...
  presence:
    enable_inbound: false
    enable_outbound: false
```
