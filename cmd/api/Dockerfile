# Build stage
FROM x-notes-builder AS builder

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates bash curl
WORKDIR /home
COPY --from=builder /src/api-server /home/
EXPOSE 8888
CMD ["/home/api-server"]
