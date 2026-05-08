FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/control-plane ./cmd/control-plane

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app
COPY --from=build /out/control-plane /usr/local/bin/control-plane

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/control-plane"]
