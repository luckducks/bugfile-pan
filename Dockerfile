FROM --platform=linux/arm64 node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=linux/arm64 golang:1.26.5-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags='-s -w' -o /out/bigfile-webdav ./cmd/bigfile-webdav

FROM alpine:3.21 AS runtime
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app -h /nonexistent app
WORKDIR /app
COPY --from=go-build /out/bigfile-webdav /usr/local/bin/bigfile-webdav
COPY --from=web-build /src/web/dist /app/web
ENV WEB_DIR=/app/web
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/bigfile-webdav"]
