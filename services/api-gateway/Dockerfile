FROM golang:1.26.3-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api-gateway ./main.go

FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
USER app
COPY --from=builder /bin/api-gateway /bin/api-gateway
EXPOSE 8080
ENTRYPOINT ["/bin/api-gateway"]
