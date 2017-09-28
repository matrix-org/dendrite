XDOCK_YAML=crossdock/docker-compose.yml

.PHONY: crossdock-linux-bin
crossdock-linux-bin:
	CGO_ENABLED=0 GOOS=linux time go build -a -installsuffix cgo -o crossdock/crossdock ./crossdock

.PHONY: crossdock
crossdock: crossdock-linux-bin
	docker-compose -f $(XDOCK_YAML) kill go
	docker-compose -f $(XDOCK_YAML) rm -f go
	docker-compose -f $(XDOCK_YAML) build go
	docker-compose -f $(XDOCK_YAML) run crossdock


.PHONY: crossdock-fresh
crossdock-fresh: crossdock-linux-bin
	docker-compose -f $(XDOCK_YAML) kill
	docker-compose -f $(XDOCK_YAML) rm --force
	docker-compose -f $(XDOCK_YAML) pull
	docker-compose -f $(XDOCK_YAML) build
	docker-compose -f $(XDOCK_YAML) run crossdock

.PHONY: crossdock-logs
crossdock-logs:
	docker-compose -f $(XDOCK_YAML) logs

.PHONY: install_docker_ci
install_docker_ci:
	@echo "Installing docker-compose $${DOCKER_COMPOSE_VERSION:?'DOCKER_COMPOSE_VERSION env not set'}"
	sudo rm -f /usr/local/bin/docker-compose
	curl -L https://github.com/docker/compose/releases/download/$${DOCKER_COMPOSE_VERSION}/docker-compose-`uname -s`-`uname -m` > docker-compose
	chmod +x docker-compose
	sudo mv docker-compose /usr/local/bin
	docker-compose version

.PHONY: crossdock_ci
crossdock_ci:
ifdef CROSSDOCK
	docker version
	$(MAKE) crossdock
else
	true
endif

.PHONY: crossdock_logs_ci
crossdock_logs_ci:
ifdef CROSSDOCK
	docker-compose -f $(XDOCK_YAML) logs
else
	true
endif

