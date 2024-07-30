FROM golang:1.22 AS builder

WORKDIR /app/src

COPY go.mod go.sum ./
RUN go mod download

COPY *.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server

FROM gcr.io/distroless/static

WORKDIR /app

COPY --from=builder /app/server .
CMD ["./server"]
