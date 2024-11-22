# syntax=docker/dockerfile:experimental

FROM golang:1.22-alpine as dev
RUN apk add --no-cache git ca-certificates
RUN adduser -D appuser
COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w -extldflags -static' -o kube-gateway .

FROM debian
COPY --from=dev /src/kube-gateway /
RUN apt-get update; apt-get install -y net-tools nftables
CMD ["/kube-gateway"]