Development with Docker
---

With `docker` and `docker-compose` you can easily spin up a development environment
and start working on dendrite.

### Requirements

- docker
- docker-compose (version 3+)

### Configuration

Create a directory named `cfg` in the root of the project. Copy the
`dendrite-docker.yaml` file into that directory and rename it to `dendrite.yaml`.
It already contains the defaults used in `docker-compose` for networking so you will
only have to change things like the `server_name` or to toggle `naffka`.

You can run the following `docker-compose` commands either from the top directory
specifying the `docker-compose` file
```
docker-compose -f docker/docker-compose.yml <cmd>
```
or from within the `docker` directory 

```
docker-compose <cmd>
```

### Starting a monolith server

For the monolith server you would need a postgres instance 

```
docker-compose up postgres
```

and the dendrite component from `bin/dendrite-monolith-server`

```
docker-compose up monolith
```

The monolith will be listening on `http://localhost:8008`.

You would also have to make the following adjustments to `dendrite.yaml`.
 - Set `use_naffka: true` 
 - Uncomment the `database/naffka` postgres url.

### Starting a multiprocess server

The multiprocess server requires kafka, zookeeper and postgres

```
docker-compose up kafka zookeeper postgres
```

and the following dendrite components 

```
docker-compose up client_api media_api sync_api room_server public_rooms_api edu_server
docker-compose up client_api_proxy
```

The `client-api-proxy` will be listening on `http://localhost:8008`.

You would also have to make the following adjustments to `dendrite.yaml`.
 - Set `use_naffka: false` 
 - Comment out the `database/naffka` postgres url.

### Starting federation

```
docker-compose up federation_api federation_sender
docker-compose up federation_api_proxy
```

You can point other Matrix servers to `http://localhost:8448`.

### Creating a new component

You can create a new dendrite component by adding an entry to the `docker-compose.yml` 
file and creating a startup script for the component in `docker/services`. 
For more information refer to the official docker-compose [documentation](https://docs.docker.com/compose/).

```yaml
  new_component:
    container_name: dendrite_room_server
    hostname: new_component
    # Start up script.
    entrypoint: ["bash", "./docker/services/new-component.sh"]
    # Use the common Dockerfile for all the dendrite components.
    build: ./
    volumes:
      - ..:/build
    depends_on:
      - another_component
    networks:
      - internal
```
