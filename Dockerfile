# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG REVISION=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" -o /out/ghcr-stats .

FROM alpine:3.22
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="ghcr-stats" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.revision="$REVISION"
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 1000 app && adduser -D -u 1000 -G app app && mkdir -p /data && chown app:app /data
COPY --from=build /out/ghcr-stats /usr/local/bin/ghcr-stats
USER 1000:1000
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -q -O - http://127.0.0.1:8080/healthz | grep -qx ok || exit 1
ENTRYPOINT ["/usr/local/bin/ghcr-stats"]
