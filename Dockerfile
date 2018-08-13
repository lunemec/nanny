FROM golang:1.10-alpine AS build

LABEL maintainer="Philip Schmid (@PhilipSchmid)"

RUN apk add build-base gcc abuild binutils binutils-doc gcc-doc
COPY ./ /go/src/nanny
WORKDIR /go/src/nanny
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /nanny .

FROM scratch

COPY --from=build /nanny ./
CMD ["./nanny"]
