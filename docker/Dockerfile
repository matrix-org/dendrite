FROM docker.io/golang:1.12.5-alpine3.9

RUN mkdir /build

WORKDIR /build

RUN apk --update --no-cache add openssl bash git

CMD ["bash", "docker/build.sh"]
