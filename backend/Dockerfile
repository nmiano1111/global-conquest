FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

# Cache dependencies before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /app/server ./cmd/backend/main.go

# ---- Runtime ----
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/server ./server
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["/app/server"]
