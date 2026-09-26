# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY srepanel/go.mod srepanel/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY srepanel/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/srepanel .

RUN mkdir /out/data && mkdir -m 1777 /out/tmp

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/tmp /tmp
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build /out/srepanel /srepanel

USER 65532:65532

WORKDIR /data
VOLUME /data
EXPOSE 8080

ENTRYPOINT ["/srepanel"]
