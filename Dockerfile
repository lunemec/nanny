FROM docker.io/library/golang:1.27.1-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "-s -w -X nanny/pkg/version.Version=${VERSION} -X nanny/pkg/version.GitCommit=${GIT_COMMIT} -X nanny/pkg/version.BuildDate=${BUILD_DATE}" -o /nanny .

FROM docker.io/library/alpine:3.24

LABEL org.opencontainers.image.source="https://github.com/lunemec/nanny" \
      org.opencontainers.image.licenses="BSD-3-Clause"

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 1000 nanny \
    && adduser -S -D -H -u 1000 -h /var/lib/nanny -s /sbin/nologin -G nanny nanny \
    && install -d -o nanny -g nanny /etc/nanny /var/lib/nanny

COPY --chmod=0755 --from=build /nanny /usr/bin/nanny
COPY --chown=nanny:nanny nanny.toml /etc/nanny/nanny.toml
RUN sed -i 's/addr="localhost:8080"/addr="0.0.0.0:8080"/' /etc/nanny/nanny.toml

USER nanny
WORKDIR /var/lib/nanny
EXPOSE 8080

ENTRYPOINT ["/usr/bin/nanny"]
CMD ["--config", "/etc/nanny/nanny.toml"]
