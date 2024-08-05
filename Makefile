all: noise-exporter

noise-exporter:
	go build -o . ./...

pi:
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	CGO_ENABLED=1 \
	CC=arm-linux-gnueabi-gcc \
	go build -o pi-noise-exporter ./...