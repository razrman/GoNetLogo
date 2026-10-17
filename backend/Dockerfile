FROM golang:1.22-bookworm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY models ./models
COPY schema ./schema
COPY backend ./backend
RUN CGO_ENABLED=0 go build -o /usr/local/bin/gonetlogo-server ./backend/cmd/server
EXPOSE 43131
ENTRYPOINT ["/usr/local/bin/gonetlogo-server", "-addr", "0.0.0.0:43131"]
