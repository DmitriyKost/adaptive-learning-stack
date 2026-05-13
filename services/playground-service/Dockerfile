FROM golang:1.26.3-alpine AS builder

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/playground-service ./main.go

FROM alpine:3.20

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/playground-service /bin/playground-service
COPY migrations ./migrations

EXPOSE 8080
ENTRYPOINT ["/bin/playground-service"]
