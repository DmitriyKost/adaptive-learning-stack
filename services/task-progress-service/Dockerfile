FROM golang:1.26.3-alpine AS builder

RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o task-progress-service ./main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/task-progress-service ./task-progress-service
EXPOSE 8082
CMD ["./task-progress-service"]
