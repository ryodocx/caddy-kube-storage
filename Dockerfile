FROM golang:1.19.0-alpine
WORKDIR /
ENV CGO_ENABLED=0
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
COPY . .
RUN xcaddy build

FROM alpine:3.16.2
COPY --from=0 /caddy /usr/local/bin/
COPY Caddyfile /etc/caddy/Caddyfile
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile"]
