###########
## Build ##
###########
FROM golang:1.13-alpine AS build
RUN apk --update --no-cache add openssl bash git ca-certificates
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOBIN=$PWD/bin go install -v ./cmd/...
