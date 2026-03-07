# Build stage
FROM x-notes-builder AS builder
ARG VERSION
ARG GIT_SHA
ARG BUILD_TIME

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
