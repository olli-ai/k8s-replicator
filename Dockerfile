FROM golang as build-stage
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY liveness liveness
COPY replicate replicate
RUN go build -o k8s-replicator

FROM alpine as production-stage
LABEL MAINTAINER="Aurelien Lambert <aure@olli-ai.com>"

RUN apk --no-cache upgrade && \
    mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
COPY --from=build-stage /app/k8s-replicator /k8s-replicator
ENTRYPOINT  ["/k8s-replicator"]
