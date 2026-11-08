FROM golang:1.27-alpine AS builder
COPY docker/gorse-launcher.go /src/main.go
RUN CGO_ENABLED=0 go build -trimpath -o /gorse-launcher /src/main.go

FROM zhenghaoz/gorse-in-one:0.5.11
COPY --from=builder /gorse-launcher /usr/local/bin/librefanza-gorse
COPY docker/gorse.toml /etc/gorse/config.toml
ENTRYPOINT ["/usr/local/bin/librefanza-gorse"]
