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

# Cross-compile for Raspberry Pi (ARM64, via Docker)
make pi

# Build and deploy to Pi (requires .env with PI_HOST, PI_USER, PI_BIN)
make deploy

# Format code
go fmt ./...

# Vet code for issues
go vet ./...

# Run tests
go test ./...
```

## Dependencies

- `github.com/gordonklaus/portaudio`: Audio I/O
- `github.com/prometheus/client_golang`: Prometheus metrics

## Configuration

Command line flags:
- `-addr`: Listen address (default `:9900`)
- `-debug`: Enable debug output
- `-list-devices`: List available audio devices and exit
- `-test-file`: Process a WAV file instead of live audio (48kHz, mono, 32-bit float)

## Deployment

- `noise-exporter.service`: Systemd unit file, deployed alongside the binary via `make deploy`
- `.env`: Gitignored file containing `PI_HOST`, `PI_USER`, and `PI_BIN` for deployment
- The service sets `JACK_NO_START_SERVER=1` to prevent PortAudio from trying to start a JACK server, which would block ALSA device enumeration on headless Pi

## Metrics

The exporter serves Prometheus metrics at `http://localhost:9900/metrics`:

| Metric | Description |
|--------|-------------|
| `noise_exporter_loudness_instant` | Instantaneous LAeq in dB |
| `noise_exporter_las_db` | A-weighted sound level, slow time weighting (1s) |
| `noise_exporter_laf_db` | A-weighted sound level, fast time weighting (125ms) |