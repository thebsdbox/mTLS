# syntax=docker/dockerfile:experimental

FROM golang:1.23-alpine as dev

COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w -extldflags -static' -o sidecar-injector ./cmd/


FROM alpine:latest

# install curl for prestop script
RUN apk --no-cache add curl

WORKDIR /

# install binary
COPY --from=dev /src/sidecar-injector .

# install the prestop script
COPY ./prestop.sh .

USER 65532:65532

ENTRYPOINT ["/sidecar-injector"]
