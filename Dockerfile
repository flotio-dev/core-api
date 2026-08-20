# syntax=docker/dockerfile:1
FROM golang:1.25.13-alpine3.23@sha256:42fc3368d1c50170a452f2bf4a1dfd292a065870c3f258d799aad4316671cb69 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .

ARG SERVICE_NAME=api
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w" -o /bin/server ./cmd/${SERVICE_NAME}

FROM alpine:3.23.5@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

ARG OCI_SOURCE
ARG OCI_REVISION
ARG OCI_VERSION
ARG OCI_CREATED

LABEL org.opencontainers.image.source=$OCI_SOURCE \
      org.opencontainers.image.revision=$OCI_REVISION \
      org.opencontainers.image.version=$OCI_VERSION \
      org.opencontainers.image.created=$OCI_CREATED

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app

COPY --from=builder /bin/server /bin/server

USER 10001:10001
EXPOSE 8080
CMD ["/bin/server"]
