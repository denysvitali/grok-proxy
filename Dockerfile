FROM golang:1.24-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/grok-proxy .

FROM alpine:3.24

RUN apk add --no-cache ca-certificates \
    && adduser -D -h /home/grok-proxy grok-proxy

COPY --from=build /out/grok-proxy /usr/local/bin/grok-proxy

USER grok-proxy
EXPOSE 8080

ENTRYPOINT ["grok-proxy"]
CMD ["serve", "--listen", "0.0.0.0:8080"]
