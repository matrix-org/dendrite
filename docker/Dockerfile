<<<<<<< HEAD
FROM docker.io/golang:1.13.7-alpine3.11
=======
FROM docker.io/golang:1.13.6-alpine
>>>>>>> master

RUN mkdir /build

WORKDIR /build

RUN apk --update --no-cache add openssl bash git build-base

CMD ["bash", "docker/build.sh"]
