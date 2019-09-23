FROM golang:1.13-alpine AS builder
WORKDIR /app
RUN export GOPRIVATE=github.com/block26/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Command to run the executable
FROM scratch
COPY --from=builder /app/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/settings /settings
COPY --from=builder /app/main /
CMD ["./main live"]