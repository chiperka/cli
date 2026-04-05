FROM golang:1.24-alpine AS build

ARG VERSION=dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X chiperka-cli/cmd.Version=${VERSION}" -o /chiperka .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates docker-cli curl bash
WORKDIR /code
COPY --from=build /chiperka /usr/local/bin/chiperka
