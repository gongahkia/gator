FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/norbot ./cmd/norbot

FROM alpine:3.21
RUN apk add --no-cache docker-cli ca-certificates
COPY --from=build /out/norbot /usr/local/bin/norbot
EXPOSE 8080
ENTRYPOINT ["norbot"]
CMD ["serve"]
