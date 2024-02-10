FROM golang:latest as builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -v -o websocket-server

FROM alpine:latest
RUN apk add --no-cache ca-certificates

COPY --from=builder /app/websocket-server /websocket-server

CMD ["/websocket-server"]
