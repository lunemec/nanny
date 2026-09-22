FROM docker.io/library/golang:1.27.1-alpine AS build

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath -tags netgo -ldflags "-s -w -X nanny/pkg/version.Version=${VERSION} -X nanny/pkg/version.GitCommit=${GIT_COMMIT} -X nanny/pkg/version.BuildDate=${BUILD_DATE}" -o /nanny .

FROM docker.io/library/alpine:3.23

RUN apk add --no-cache ca-certificates

RUN adduser -s /sbin/nologin -u 1000 -H -h /opt -D nanny

RUN mkdir -p /opt

COPY --chown=1000:1000 --from=build /nanny /opt/
COPY --chown=1000:1000 nanny.toml /opt/
RUN sed -i 's/addr="localhost:8080"/addr="0.0.0.0:8080"/g' /opt/nanny.toml
RUN sed -i -r 's/storage_dsn="file:nanny.sqlite".*/storage_dsn="file:\/opt\/nanny.sqlite"/g' /opt/nanny.toml
RUN chown -R nanny:nanny /opt

USER nanny
EXPOSE 8080

ENTRYPOINT ["/opt/nanny"]
CMD ["--config", "/opt/nanny.toml"]
