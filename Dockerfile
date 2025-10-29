FROM cgr.dev/chainguard/go:latest AS builder
ARG APP
ENV CGO_ENABLED=0
ENV GOOS=linux

WORKDIR /src
COPY go.* ./
RUN go mod download
COPY . .

RUN go test -v ./...
RUN go run honnef.co/go/tools/cmd/staticcheck@latest ./...
RUN go run golang.org/x/vuln/cmd/govulncheck@latest ./...
RUN go run golang.org/x/tools/cmd/deadcode@latest -test ./...
RUN go build -a -installsuffix cgo -o ./bin/app ./cmd/$APP

FROM cgr.dev/chainguard/static
COPY --from=builder /src/bin/app /app
ENTRYPOINT ["/app"]
