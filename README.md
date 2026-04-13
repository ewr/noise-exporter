# noise-exporter

A Prometheus exporter for noise monitoring. Reads audio input from a USB sound level meter (assuming 32-bit float values calibrated in Pa), converts to decibels, and exposes metrics for Prometheus scraping.

## Metrics

| Metric | Description |
|--------|-------------|
| `noise_exporter_loudness_instant` | Instantaneous LAeq in dB |
| `noise_exporter_las_db` | A-weighted sound level, slow time weighting (1s) |
| `noise_exporter_laf_db` | A-weighted sound level, fast time weighting (125ms) |

## Usage

```bash
# Build
make

# Cross-compile for Raspberry Pi (ARM64, via Docker)
make pi

# Build and deploy to Pi
make deploy
```

Deployment config (host, user, binary path) is read from a `.env` file — see the Makefile for the expected variables.

```bash
# Run
./noise-exporter

# Run with debug output
./noise-exporter -debug

# List available audio devices
./noise-exporter -list-devices

# Test with a WAV file instead of live audio
./noise-exporter -test-file recording.wav
```

The WAV file must be 48kHz, mono, 32-bit float (IEEE).

## Deployment

The repo includes a `noise-exporter.service` systemd unit file. `make deploy` cross-compiles for ARM64, copies the binary and service file to the Pi, and restarts the service.

## Dependencies

- [PortAudio](http://www.portaudio.com/) (`portaudio19-dev`) for audio capture
- [Prometheus client_golang](https://github.com/prometheus/client_golang) for metrics