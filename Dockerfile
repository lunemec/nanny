FROM docker.io/library/golang:1.15.8-alpine AS build

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

RUN apk add --no-cache build-base gcc abuild binutils binutils-doc gcc-doc
COPY ./ /go/src/nanny
WORKDIR /go/src/nanny
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /nanny .

FROM docker.io/library/alpine:3.13

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