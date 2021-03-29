FROM golang:1.16.2-alpine3.13 AS build_base

RUN apk add --no-cache git

# Set the Current Working Directory inside the container
WORKDIR /tmp/app

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go get -u github.com/gobuffalo/packr/v2/packr2

COPY . .

# Build the Go app
RUN packr2
RUN go build

# Start fresh from a smaller image
FROM alpine:3.9 
RUN apk add ca-certificates

WORKDIR /app

COPY --from=build_base /tmp/app/api.poll.komodohype.dev /app/api.poll.komodohype.dev
COPY config.yaml /app/config.yaml

# This container exposes port 8080 to the outside world
EXPOSE 8080

# Run the binary program produced by `go install`
CMD ["/app/api.poll.komodohype.dev"]
