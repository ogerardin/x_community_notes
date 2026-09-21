# Build stage
FROM golang:1.26-alpine AS builder
ARG VERSION
ARG GIT_SHA
ARG BUILD_TIME

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=${VERSION} -X main.GitSHA=${GIT_SHA} -X main.BuildTime=${BUILD_TIME}" -o api-server

# Runtime stage
FROM alpine:3.21
ARG VERSION
ARG GIT_SHA
ARG BUILD_TIME

LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.created="${BUILD_TIME}"

RUN apk --no-cache add ca-certificates bash curl
WORKDIR /home
COPY --from=builder /src/api-server /home/
EXPOSE 8888
CMD ["/home/api-server"]
