# syntax=docker/dockerfile:1.7
#
# Hot-reload image for Go services. Used only by the `watch` profile in
# docker-compose.yml — production builds still use the per-service
# Dockerfiles next to this one.
#
# The container expects:
#   - Source bind-mounted at /app
#   - $GOMODCACHE / $GOCACHE on named volumes (so caches survive
#     container recreates AND restarts of the host machine)
#   - $WARMBLY_CMD set to one of: backend, consumer, worker, seed
#
# air watches for changes under /app and rebuilds the cmd binary into
# /tmp/main in place, then restarts it. No docker layer pipeline, no
# image rebuild — just `go build` against a warm cache.

FROM golang:1.26-alpine

RUN apk add --no-cache git ca-certificates gcc musl-dev librdkafka-dev curl

# Pin air so the dev image is reproducible. air-verse/air is the
# successor to cosmtrek/air (same project, new org).
RUN go install github.com/air-verse/air@v1.61.5

ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build
ENV GOTMPDIR=/tmp
ENV CGO_ENABLED=1

WORKDIR /app

COPY deploy/docker/air.toml /etc/air.toml

ENTRYPOINT ["air", "-c", "/etc/air.toml"]
