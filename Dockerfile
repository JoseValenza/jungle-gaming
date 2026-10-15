
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /jungle-gaming ./cmd

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /jungle-gaming /app/jungle-gaming

EXPOSE 8080

CMD ["/app/jungle-gaming"]
