FROM golang:alpine3.6

RUN mkdir /build

WORKDIR /build

RUN apk --update --no-cache add openssl bash git && \
    go get github.com/constabulary/gb/...

CMD ["bash", "docker/build.sh"]
