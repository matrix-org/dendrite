PACKAGES := $(shell glide novendor)

export GO15VENDOREXPERIMENT=1


.PHONY: build
build:
	go build $(PACKAGES)


.PHONY: install
install:
	glide --version || go get github.com/Masterminds/glide
	glide install


.PHONY: test
test:
	go test $(PACKAGES)


.PHONY: cover
cover:
	./scripts/cover.sh $(shell go list $(PACKAGES))
	go tool cover -html=cover.out -o cover.html


.PHONY: install_ci
install_ci: install
	go get -u -f github.com/golang/lint/golint
	go get github.com/wadey/gocovmerge
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover


.PHONY: test_ci
test_ci:
	./scripts/cover.sh $(shell go list $(PACKAGES))

.PHONY: lint_ci
lint_ci:
	golint $(PACKAGES)
	go vet $(PACKAGES)
