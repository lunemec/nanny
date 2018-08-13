FROM golang:1.10-alpine AS build

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

RUN adduser -s /sbin/nologin -H -u 1000 -D nanny

RUN apk add build-base gcc abuild binutils binutils-doc gcc-doc
COPY ./ /go/src/nanny
WORKDIR /go/src/nanny
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /nanny .

FROM scratch

COPY --chown=1000:1000 --from=build /nanny ./
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/group /etc/group

# Does not work yet:
#USER nanny
EXPOSE 8080

CMD ["./nanny"]
