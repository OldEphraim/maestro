FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /maestro ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /maestro /maestro
COPY migrations/ /migrations/
COPY templates/ /templates/

ENV PORT=8080
EXPOSE 8080
CMD ["/maestro"]
