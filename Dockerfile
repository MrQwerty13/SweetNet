FROM node:26.0.0-bookworm-slim AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.4-alpine3.23 AS go-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/admin ./cmd/admin

FROM alpine:3.23.3 AS runtime
RUN addgroup -g 10001 sweetnet \
    && adduser -D -H -u 10001 -G sweetnet sweetnet \
    && mkdir -p /app/web /data/uploads \
    && chown 10001:10001 /data/uploads \
    && chmod 0700 /data/uploads
WORKDIR /app
COPY --from=go-build /out/server /out/admin /app/
COPY --from=web-build /src/web/dist/ /app/web/
USER 10001:10001
ENV HTTP_ADDR=:8080 UPLOAD_DIR=/data/uploads WEB_DIR=/app/web
EXPOSE 8080
CMD ["/app/server"]
