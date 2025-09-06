# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based Prometheus exporter for noise monitoring. It reads audio input (assuming 32-bit float values calibrated in Pa), converts to decibels, and exposes noise metrics via HTTP endpoint for Prometheus scraping.

## Architecture

- **Single binary**: All code is in `src/noise-exporter/main.go`
- **Audio processing**: Uses PortAudio for real-time audio capture
- **Metrics**: Prometheus client library for metric exposition
- **Real-time processing**: Audio callback function processes samples and updates metrics continuously

Key components:
- Audio stream setup with 48kHz sample rate, mono channel
- Decibel calculation using reference pressure of 20µPa
- Prometheus gauge metric `noise_exporter_loudness_instant` for LAeq readings
- HTTP server on port 9900 (configurable) serving `/metrics` endpoint

## Build Commands

```bash
# Build for current platform
make
# or
go build -o . ./...

# Cross-compile for Raspberry Pi
make pi
```

## Development Commands

```bash
# Run the exporter
./noise-exporter

# Run with debug output
./noise-exporter -debug

# Run on different port
./noise-exporter -addr :8080

# Format code
go fmt ./...

# Vet code for issues
go vet ./...

# Run tests (if any exist)
go test ./...

# Clean build artifacts
go clean
```

## Dependencies

- `github.com/gordonklaus/portaudio`: Audio I/O
- `github.com/prometheus/client_golang`: Prometheus metrics

## Configuration

Command line flags:
- `-addr`: Listen address (default `:9900`)
- `-debug`: Enable debug output

## Metrics Endpoint

The exporter serves Prometheus metrics at `http://localhost:9900/metrics` with the metric `noise_exporter_loudness_instant` representing the current LAeq value in decibels.