# syntax=docker/dockerfile:1
FROM golang:1.27.1-alpine3.23@sha256:d9e2f2f07b10cc922da3e80e035c3058810b328d5aef82d2c63680967c5e2ec9 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .

ARG SERVICE_NAME=api
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" -o /bin/server ./cmd/${SERVICE_NAME}

FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ARG OCI_SOURCE
ARG OCI_REVISION
ARG OCI_VERSION
ARG OCI_CREATED

LABEL org.opencontainers.image.source=$OCI_SOURCE \
      org.opencontainers.image.revision=$OCI_REVISION \
      org.opencontainers.image.version=$OCI_VERSION \
      org.opencontainers.image.created=$OCI_CREATED

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

COPY --from=builder /bin/server /bin/server

USER 10001:10001
EXPOSE 8080
CMD ["/bin/server"]
