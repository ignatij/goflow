FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o /goflow ./cmd/goflow/main.go

FROM scratch
COPY --from=builder /goflow /goflow
# Set the entrypoint to the Go binary
ENTRYPOINT ["/goflow"]