FROM golang as build-stage
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY liveness liveness
COPY replicate replicate
RUN go build -o k8s-replicator

FROM golang as production-stage
LABEL MAINTAINER="Aurelien Lambert <aure@olli-ai.com>"

COPY --from=build-stage /app/k8s-replicator /k8s-replicator
ENTRYPOINT  ["/k8s-replicator"]
