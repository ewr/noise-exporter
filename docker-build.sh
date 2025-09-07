# Build the container
docker build -t go-arm-cross .

# Use it to cross-compile
docker run --rm -v $(pwd):/src go-arm-cross \
  env GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
  go build -o pi-noise-exporter ./...