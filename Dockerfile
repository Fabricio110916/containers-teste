FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN go build -o proxy main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/proxy /app/proxy

# Porta usada pelo seu servidor Go
EXPOSE 8080

CMD ["./proxy"]
