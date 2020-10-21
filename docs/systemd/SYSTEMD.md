## Systemd setup and usage

#### Manual install
* Edit `docs/systemd/*.service` and copy the file to `/etc/systemd/system/`
* Reload systemd: `systemctl daemon-reload`

_Monolith:_
* Enable the service with: `systemctl enable monolith-example.service`
* Start the service: `systemctl start monolith-example.service`

_Polylith:_
* Enable the services with: `systemctl enable polylith-example@<server>.service` (repeat for each server)
* Start the services: `systemctl start polylith-example@*.service` 

___
#### Scripted install (Polylith)
In order to run the whole polylith deployment daemonized with Systemd
the following script can set them up for you **(root-privileges required)**:
```bash
# ./install-polylith-systemd-units.sh
```


Example:

```bash
# ./install-polylith-systemd-units.sh -u dendrite -d /home/dendrite/server/ all
```
* user: dendrite
* path: /opt/dendrite/
* selection: all

How to start a specific service (e.g clientapi):
```bash
# systemctl start dendrite@clientapi
```

...or all services using a wildcard:
```bash
# systemctl start dendrite@*
```

Monitor with journalctl:

```bash
# journalctl -fu dendrite@clientapi
```
or

```bash
# journalctl -fu dendrite@*
```