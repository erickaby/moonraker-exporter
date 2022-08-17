FROM golang:1.18 AS build

LABEL org.opencontainers.image.source https://github.com/erickaby/moonraker-exporter
LABEL maintainer="Ethan Rickaby <ejrickaby@gmail.com>"

WORKDIR /go/src/moonraker-exporter

# Copy all the Code and stuff to compile everything
COPY . .

# Downloads all the dependencies in advance (could be left out, but it's more clear this way)
RUN go mod download

# Builds the application as a staticly linked one, to allow it to run on alpine
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o app .


# Moving the binary to the 'final Image' to make it smaller
FROM alpine:latest

WORKDIR /app

# Create the `public` dir and copy all the assets into it
# RUN mkdir ./static
# COPY ./static ./static

# `boilerplate` should be replaced here as well
COPY --from=build /go/src/moonraker-exporter/app .

# Exposes port 3000 because our program listens on that port
EXPOSE 9101

CMD ["./app"]