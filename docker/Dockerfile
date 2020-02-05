FROM docker.io/golang:1.13.6-alpine

RUN mkdir /build

WORKDIR /build

RUN apk --update --no-cache add openssl bash git

CMD ["bash", "docker/build.sh"]
