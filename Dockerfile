FROM docker.io/library/golang:1.15.8-alpine AS build

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

RUN apk add --no-cache build-base gcc abuild binutils binutils-doc gcc-doc
COPY ./ /go/src/nanny
WORKDIR /go/src/nanny
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /nanny .

FROM docker.io/library/alpine:3.13

RUN adduser -s /sbin/nologin -H -u 1000 -D nanny
RUN mkdir -p /opt
RUN chown nanny:nanny /opt

RUN apk add --no-cache ca-certificates

WORKDIR /opt

COPY --chown=1000:1000 --from=build /nanny /opt/

USER nanny
EXPOSE 8080

CMD ["/opt/nanny"]
