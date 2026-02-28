# Build stage
FROM x-notes-builder AS builder
ARG VERSION
ARG GIT_SHA
ARG BUILD_TIME

# Runtime stage
FROM alpine:latest
ARG VERSION
ARG GIT_SHA
ARG BUILD_TIME

RUN apk --no-cache add ca-certificates bash curl
WORKDIR /home
COPY --from=builder --build-arg VERSION=${VERSION} --build-arg GIT_SHA=${GIT_SHA} --build-arg BUILD_TIME=${BUILD_TIME} /src/api-server /home/
EXPOSE 8888
CMD ["/home/api-server"]
