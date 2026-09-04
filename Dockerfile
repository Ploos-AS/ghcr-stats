# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG REVISION=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ghcr-stats .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 1000 app && adduser -D -u 1000 -G app app && mkdir -p /data && chown app:app /data
COPY --from=build /out/ghcr-stats /usr/local/bin/ghcr-stats
USER 1000:1000
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ghcr-stats"]
