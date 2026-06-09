# ---- stage 1: build ----
FROM golang:1.23-alpine AS go-build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/bismuth ./cmd/bismuth

# ---- stage 2: web ----
FROM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# ---- stage 3: runtime ----
FROM alpine:3.21
RUN apk add --no-cache ca-certificates git bash
COPY --from=go-build /bin/bismuth /usr/local/bin/bismuth
COPY --from=web-build /web/dist /opt/bismuth/web/dist

# data dir for SQLite
RUN mkdir -p /data
VOLUME /data

EXPOSE 9000
ENTRYPOINT ["bismuth"]
CMD ["serve", "--config", "/etc/bismuth/config.yaml"]
