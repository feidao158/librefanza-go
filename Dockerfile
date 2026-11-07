FROM node:24-alpine AS frontend
WORKDIR /work/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend
WORKDIR /work/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOMAXPROCS=2 go build -p 2 -trimpath -o /out/librefanza ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata chromium && addgroup -S app && adduser -S -G app app && mkdir -p /data /app/frontend && chown -R app:app /data /app
COPY --from=backend /out/librefanza /app/librefanza
COPY --from=frontend /work/frontend/dist/ /app/frontend/
USER app
WORKDIR /app
ENV LIBREFANZA_DATA_DIR=/data LIBREFANZA_FRONTEND_DIR=/app/frontend LIBREFANZA_LISTEN=:8080
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/librefanza"]
