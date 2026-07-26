FROM node:22-alpine AS web-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web-build /web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/norbot ./cmd/norbot

FROM alpine:3.21
RUN apk add --no-cache docker-cli git ca-certificates
COPY --from=build /out/norbot /usr/local/bin/norbot
EXPOSE 8080
ENTRYPOINT ["norbot"]
CMD ["serve"]
