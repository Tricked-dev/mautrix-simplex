FROM golang:1.25-bookworm AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "\
      -X main.Tag=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildTime=${BUILD_TIME}" \
    -o /mautrix-simplex ./cmd/mautrix-simplex/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /mautrix-simplex /usr/bin/mautrix-simplex

VOLUME /data
WORKDIR /data
ENV HOME=/data

EXPOSE 29340

CMD ["/usr/bin/mautrix-simplex", "-c", "/data/config.yaml"]
