# kvist server image. Themes are embedded in the binary.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY themes ./themes
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/kvist ./cmd/kvist

FROM alpine:3.20
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
