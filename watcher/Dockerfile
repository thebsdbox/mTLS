# syntax=docker/dockerfile:experimental

FROM golang:1.23-alpine as dev
RUN apk add --no-cache git ca-certificates upx
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w -extldflags -static' -o kube-cert-watcher .
RUN upx ./kube-cert-watcher
#FROM debian
FROM scratch
COPY --from=dev /src/kube-cert-watcher /
#RUN apt-get update; apt-get install -y net-tools nftables
CMD ["/kube-cert-watcher"]