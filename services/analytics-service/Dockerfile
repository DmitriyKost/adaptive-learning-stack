FROM golang:1.26.3-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/analytics-service ./main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /out/analytics-service /app/analytics-service
COPY migrations ./migrations

EXPOSE 8083

CMD ["/app/analytics-service"]
