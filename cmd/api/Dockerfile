FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY migrations/ ./migrations/
RUN CGO_ENABLED=0 GOOS=linux go build -o api-server

FROM alpine:latest
RUN apk --no-cache add ca-certificates bash curl
WORKDIR /home
COPY --from=builder /src/api-server /home/
EXPOSE 8888
CMD ["/home/api-server"]
