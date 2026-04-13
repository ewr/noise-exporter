include .env

all: noise-exporter

noise-exporter:
	go build -o . ./...

clean:
	rm -f noise-exporter pi-noise-exporter

pi:
	docker build -t go-arm-cross .
	docker run --rm -v $(PWD):/src go-arm-cross \
		env GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
		go build -o pi-noise-exporter ./...

deploy: pi
	scp pi-noise-exporter $(PI_USER)@$(PI_HOST):/tmp/noise-exporter
	scp noise-exporter.service $(PI_USER)@$(PI_HOST):/tmp/noise-exporter.service
	ssh $(PI_USER)@$(PI_HOST) "\
		sudo systemctl stop noise-exporter && \
		sudo cp /tmp/noise-exporter $(PI_BIN) && \
		sudo cp /tmp/noise-exporter.service /etc/systemd/system/noise-exporter.service && \
		sudo systemctl daemon-reload && \
		sudo systemctl start noise-exporter"
