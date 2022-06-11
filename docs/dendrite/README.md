# Dendrite configuration files

This directory contains some Dendrite configuration files to be taken as examples.

| File                            | Use                                                                                                                |
|---------------------------------|--------------------------------------------------------------------------------------------------------------------|
| `dendrite-sample.monolith.yaml` | Dendrite executed in the mode where all functionalities are implemented by a single process (*monolith*).          |
| `dendrite-sample.polylith.yaml` | Dendrite executed in the mode where each main functionality is implemented by an independent process (*polylith*). |

It is also possible to generate a generic configuration file from scratch using the command `generate-config`.

* Linux or *nix-like OS
  ```sh
  bin/generate-config -server example.com -db postgres://user:password@dbhost:5432/dbname?sslmode=disable > dendrite.yaml
  ```
* Windows
  ```dos
  generate-config.exe -server example.com -db postgres://user:password@dbhost:5432/dbname?sslmode=disable > dendrite.yaml
  ```
