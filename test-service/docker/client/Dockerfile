FROM golang:1.24 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o client ./cmd/client.go

FROM scratch
WORKDIR /app

COPY --from=builder /app/client .

CMD ["./client"]

