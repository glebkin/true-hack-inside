FROM golang:1.24 as builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN make build

FROM alpine:latest
WORKDIR /app

COPY --from=builder /app/true-hack .
COPY --from=builder /app/configs/config.yaml .
COPY --from=builder /app/configs/.token_key .
COPY --from=builder /app/static ./static

RUN apk --no-cache add ca-certificates

CMD ["./true-hack"]

