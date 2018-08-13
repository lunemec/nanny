FROM golang:1.10-alpine AS build

ARG NANNY_PACKAGE="untagged-018d21470eb2f6c9ef3d"
ARG NANNY_PACKAGE_FILETYPE="tar.gz"

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

RUN apk add build-base gcc abuild binutils binutils-doc gcc-doc
ADD https://github.com/lunemec/nanny/archive/$NANNY_PACKAGE.$NANNY_PACKAGE_FILETYPE /go/src/$NANNY_PACKAGE.$NANNY_PACKAGE_FILETYPE
WORKDIR /go/src
RUN tar xzf $NANNY_PACKAGE.$NANNY_PACKAGE_FILETYPE
RUN mv nanny-$NANNY_PACKAGE nanny
WORKDIR /go/src/nanny
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /nanny .

FROM scratch

COPY --from=build /nanny ./
CMD ["./nanny"]
