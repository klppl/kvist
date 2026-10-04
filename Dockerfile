# kvist server image. Themes are embedded in the binary.
# The build stage runs on the build machine and cross-compiles, so
# multi-arch images (amd64, arm64) build without emulating Go.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY themes ./themes
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/kvist ./cmd/kvist

FROM alpine:3.24
LABEL org.opencontainers.image.source="https://github.com/klppl/kvist" \
      org.opencontainers.image.description="kvist: publish an Obsidian vault as a digital garden" \
      org.opencontainers.image.licenses="LicenseRef-Lagom"
RUN adduser -D -H -u 10001 kvist \
 && mkdir -p /data /etc/kvist \
 && chown kvist:kvist /data
COPY --from=build /out/kvist /usr/local/bin/kvist
USER kvist
VOLUME ["/data"]
ENV KVIST_CONFIG=/etc/kvist/kvist.toml
EXPOSE 8080
ENTRYPOINT ["kvist"]
CMD ["serve"]
