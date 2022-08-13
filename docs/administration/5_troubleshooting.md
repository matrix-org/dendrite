---
title: Troubleshooting
parent: Administration
permalink: /administration/troubleshooting
---

# Troubleshooting

If your Dendrite installation is acting strangely, there are a few things you should
check before seeking help.

## 1. Logs

Dendrite, by default, will log all warnings and errors to stdout, in addition to any
other locations configured in the `dendrite.yaml` configuration file. Often there will
be clues in the logs.

You can increase this log level to the more verbose `debug` level if necessary by adding
this to the config and restarting Dendrite:

```
logging:
- type: std
  level: debug
```

Look specifically for lines that contain `level=error` or `level=warning`.

## 2. Federation tester

If you are experiencing problems federating with other homeservers, you should check
that the [Federation Tester](https://federationtester.matrix.org) is passing for your
server.

Common reasons that it may not pass include:

1. Incorrect DNS configuration;
2. Misconfigured DNS SRV entries or well-known files;
3. Invalid TLS/SSL certificates;
4. Reverse proxy configuration issues (if applicable).

Correct any errors if shown and re-run the federation tester to check the results.

## 3. System time

Matrix relies heavily on TLS which requires the system time to be correct. If the clock
drifts then you may find that federation no works reliably (or at all) and clients may
struggle to connect to your Dendrite server.

Ensure that your system time is correct and consider syncing to a reliable NTP source.

## 4. Database connections

If you are using the PostgreSQL database, you should ensure that Dendrite's configured
number of database connections does not exceed the maximum allowed by PostgreSQL.

Open your `postgresql.conf` configuration file and check the value of `max_connections`
(which is typically `100` by default). Then open your `dendrite.yaml` configuration file
and ensure that:

1. If you are using the `global.database` section, that `max_open_conns` does not exceed
   that number;
2. If you are **not** using the `global.database` section, that the sum total of all
   `max_open_conns` across all `database` blocks does not exceed that number.

## 5. File descriptors

Dendrite requires a sufficient number of file descriptors for every connection it makes
to a remote server, every connection to the database engine and every file it is reading
or writing to at a given time (media, logs etc). We recommend ensuring that the limit is
no lower than 65535 for Dendrite.

Dendrite will check at startup if there are a sufficient number of available descriptors.
If there aren't, you will see a log lines like this:

```
level=warning msg="IMPORTANT: Process file descriptor limit is currently 65535, it is recommended to raise the limit for Dendrite to at least 65535 to avoid issues"
```

Follow the [Optimisation](../installation/11_optimisation.md) instructions to correct the
available number of file descriptors.

## 6. STUN/TURN Server tester

If you are experiencing problems with VoIP or video calls, you should check that Dendrite
is able to successfully connect your TURN server using 
[Matrix VoIP Tester](https://test.voip.librepush.net/). This can highlight any issues
that the server may encounter so that you can begin the troubleshooting process.
