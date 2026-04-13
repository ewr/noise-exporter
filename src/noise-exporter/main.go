package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	flagAddr       = flag.String("addr", ":9900", "Listen on address")
	flagDebug      = flag.Bool("debug", false, "Print debug messages")
	flagListDevices = flag.Bool("list-devices", false, "List available audio devices and exit")
	flagTestFile   = flag.String("test-file", "", "Test with WAV file instead of live audio input")
)

type metrics struct {
	lAeq prometheus.Gauge
	lAS  prometheus.Gauge
	lAF  prometheus.Gauge
}

const sampleRate = 48000
const channels = 1
const bufferSeconds = 30
const pRef = 20e-6

// A-weighting filter coefficients for 48kHz sample rate
// Based on IEC 61672-1 standard
type AWeightingFilter struct {
	x1, x2, y1, y2 float64
}

func newAWeightingFilter() *AWeightingFilter {
	return &AWeightingFilter{}
}

// Process one sample through A-weighting filter
func (f *AWeightingFilter) process(input float64) float64 {
	// Simplified A-weighting approximation for 48kHz sample rate
	// This is a basic high-pass + low-pass combination that approximates A-weighting
	// For production use, consider a more accurate multi-stage implementation

	// High-pass component (attenuates low frequencies)
	cutoffHigh := 2 * math.Pi * 20.6 / sampleRate // ~20Hz cutoff
	alphaHigh := 1.0 / (1.0 + cutoffHigh)

	// Apply simple first-order high-pass
	highpass := input - f.x1 + alphaHigh*f.y1
	f.x1 = input
	f.y1 = highpass

	// For now, return the high-pass filtered signal
	// This gives basic A-weighting behavior (attenuates low frequencies)
	return highpass
}

// Time weighting for sound level meters
type TimeWeighting struct {
	timeConstant float64 // original time constant in seconds
	average      float64 // current average
	initialized  bool
}

func newTimeWeighting(timeConstant float64) *TimeWeighting {
	return &TimeWeighting{timeConstant: timeConstant}
}

func (tw *TimeWeighting) process(input float64, bufferSize int) float64 {
	if !tw.initialized {
		tw.average = input
		tw.initialized = true
	} else {
		// Calculate alpha based on actual buffer size and time constant
		// This ensures the time constant is independent of buffer size
		bufferTime := float64(bufferSize) / sampleRate
		alpha := math.Exp(-bufferTime / tw.timeConstant)
		tw.average = alpha*tw.average + (1-alpha)*input
	}
	return tw.average
}

// WAV file header structure
type WAVHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
	Subchunk2ID   [4]byte
	Subchunk2Size uint32
}

// Read and parse WAV file header
func readWAVHeader(file *os.File) (*WAVHeader, error) {
	header := &WAVHeader{}
	err := binary.Read(file, binary.LittleEndian, header)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV header: %v", err)
	}

	// Validate WAV format
	if string(header.ChunkID[:]) != "RIFF" {
		return nil, fmt.Errorf("not a valid RIFF file")
	}
	if string(header.Format[:]) != "WAVE" {
		return nil, fmt.Errorf("not a valid WAVE file")
	}
	if string(header.Subchunk1ID[:]) != "fmt " {
		return nil, fmt.Errorf("invalid format chunk")
	}
	if string(header.Subchunk2ID[:]) != "data" {
		return nil, fmt.Errorf("invalid data chunk")
	}

	return header, nil
}

// Process WAV file in chunks and update metrics
func processWAVFile(filename string, m *metrics) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open WAV file: %v", err)
	}
	defer file.Close()

	header, err := readWAVHeader(file)
	if err != nil {
		return err
	}

	log.Printf("WAV file info:")
	log.Printf("  Sample rate: %d Hz", header.SampleRate)
	log.Printf("  Channels: %d", header.NumChannels)
	log.Printf("  Bits per sample: %d", header.BitsPerSample)
	log.Printf("  Audio format: %d (3=IEEE float)", header.AudioFormat)
	log.Printf("  Data size: %d bytes", header.Subchunk2Size)

	// Validate format matches our expectations
	if header.SampleRate != sampleRate {
		return fmt.Errorf("sample rate mismatch: expected %d, got %d", sampleRate, header.SampleRate)
	}
	if header.NumChannels != channels {
		return fmt.Errorf("channel count mismatch: expected %d, got %d", channels, header.NumChannels)
	}
	if header.AudioFormat != 3 {
		return fmt.Errorf("expected IEEE float format (3), got %d", header.AudioFormat)
	}
	if header.BitsPerSample != 32 {
		return fmt.Errorf("expected 32-bit samples, got %d", header.BitsPerSample)
	}

	// Initialize filters
	aFilter := newAWeightingFilter()
	slowWeighting := newTimeWeighting(1.0)   // 1 second for LAS
	fastWeighting := newTimeWeighting(0.125) // 0.125 second for LAF

	// Process in chunks similar to audio callback
	chunkSize := 417 // samples per chunk (realistic device buffer size)
	chunk := make([]float32, chunkSize)
	chunkCount := 0
	totalSamples := int(header.Subchunk2Size) / 4 // 4 bytes per float32

	log.Printf("Processing %d samples in chunks of %d...", totalSamples, chunkSize)

	for {
		// Read chunk by chunk using io.ReadFull for exact control
		bytesToRead := chunkSize * 4 // 4 bytes per float32
		buffer := make([]byte, bytesToRead)
		n, err := io.ReadFull(file, buffer)

		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			// Handle partial read at end of file
			if n == 0 {
				break
			}
			// Adjust chunk size for partial read
			actualSamples := n / 4
			chunk = make([]float32, actualSamples)
			bytesToRead = actualSamples * 4
			buffer = buffer[:bytesToRead]
		} else if err != nil {
			return fmt.Errorf("failed to read audio data: %v", err)
		}

		// Convert bytes to float32 samples
		for i := 0; i < len(buffer)/4; i++ {
			bits := binary.LittleEndian.Uint32(buffer[i*4 : (i+1)*4])
			chunk[i] = math.Float32frombits(bits)
		}

		chunkCount++

		// Process this chunk exactly like the audio callback
		processAudioChunk(chunk, m, aFilter, slowWeighting, fastWeighting, chunkCount)

		// Simulate real-time processing
		actualChunkSize := len(chunk)
		time.Sleep(time.Duration(float64(actualChunkSize)/float64(sampleRate)*1000) * time.Millisecond)

		// Reset chunk size for next iteration
		if len(chunk) != chunkSize {
			chunk = make([]float32, chunkSize)
		}
	}

	log.Printf("Finished processing WAV file: %d chunks, %d total samples", chunkCount, totalSamples)
	return nil
}

// Extract audio processing logic into a separate function
func processAudioChunk(in []float32, m *metrics, aFilter *AWeightingFilter, slowWeighting, fastWeighting *TimeWeighting, chunkCount int) {
	if *flagDebug && chunkCount%100 == 1 {
		log.Printf("Processing chunk #%d: %d samples", chunkCount, len(in))
	}

	// Original instant measurement (keep for compatibility)
	decibels := make([]float64, len(in))
	var sum float64 = 0
	for i := range in {
		decibels[i] = 20 * math.Log10(math.Abs(float64(in[i]))/pRef)
		sum += float64(decibels[i])
	}
	m.lAeq.Set(sum / float64(len(in)))

	// Proper A-weighted sound level calculation
	var squaredSum float64 = 0
	maxSample := float64(0)
	for _, sample := range in {
		// Track max sample for debugging
		absSample := math.Abs(float64(sample))
		if absSample > maxSample {
			maxSample = absSample
		}

		// Apply A-weighting filter
		weighted := aFilter.process(float64(sample))
		// Square the weighted pressure value
		squaredSum += weighted * weighted
	}

	// Calculate RMS of the A-weighted signal
	rms := math.Sqrt(squaredSum / float64(len(in)))

	// Use a noise floor to handle very quiet signals
	noiseFloor := 1e-5 * pRef
	if rms < noiseFloor {
		rms = noiseFloor
	}

	// Convert to decibels
	instantLA := 20 * math.Log10(rms/pRef)

	// Apply time weighting
	slowLA := slowWeighting.process(instantLA, len(in))
	fastLA := fastWeighting.process(instantLA, len(in))

	// Update metrics
	m.lAS.Set(slowLA)
	m.lAF.Set(fastLA)

	if *flagDebug {
		if chunkCount%200 == 1 {
			log.Printf("Audio: max=%.6f, rms=%.6f (%.1f dB), LAS: %.1f dB, LAF: %.1f dB",
				maxSample, rms, instantLA, slowLA, fastLA)
		}
	}
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		lAeq: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "noise_exporter_loudness_instant",
			Help: "Latest loudness reading (LAeq)",
		}),
		lAS: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "noise_exporter_las_db",
			Help: "A-weighted sound level with slow time weighting (LAS) in dB",
		}),
		lAF: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "noise_exporter_laf_db",
			Help: "A-weighted sound level with fast time weighting (LAF) in dB",
		}),
	}
	reg.MustRegister(m.lAeq, m.lAS, m.lAF)
	return m
}

func main() {
	flag.Parse()

	registry := prometheus.NewRegistry()
	m := newMetrics(registry)

	log.Printf("Initializing PortAudio...")
	err := portaudio.Initialize()
	if err != nil {
		log.Fatalf("Failed to initialize PortAudio: %v", err)
	}
	defer portaudio.Terminate()
	log.Printf("PortAudio initialized successfully")

	// Always print available audio devices for debugging
	log.Printf("Enumerating audio devices...")
	devices, err := portaudio.Devices()
	if err != nil {
		log.Printf("Warning: Failed to get devices: %v", err)
	} else {
		log.Printf("Found %d audio devices:", len(devices))
		for i, device := range devices {
			log.Printf("  Device %d: %s", i, device.Name)
			if *flagListDevices || *flagDebug {
				log.Printf("    Max input channels: %d", device.MaxInputChannels)
				log.Printf("    Max output channels: %d", device.MaxOutputChannels)
				log.Printf("    Default sample rate: %.0f Hz", device.DefaultSampleRate)
				log.Printf("    Low input latency: %.3f ms", device.DefaultLowInputLatency.Seconds()*1000)
				log.Printf("    High input latency: %.3f ms", device.DefaultHighInputLatency.Seconds()*1000)
				if device.HostApi != nil {
					log.Printf("    Host API: %s", device.HostApi.Name)
				}
			}
		}
	}

	// Always print default devices for debugging
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		log.Printf("Warning: No default input device: %v", err)
	} else {
		log.Printf("Default input device: %s", defaultInput.Name)
		log.Printf("  Max input channels: %d", defaultInput.MaxInputChannels)
		log.Printf("  Default sample rate: %.0f Hz", defaultInput.DefaultSampleRate)
	}

	// If just listing devices, exit here
	if *flagListDevices {
		log.Printf("Device listing complete - exiting")
		return
	}

	// Check if we're in test mode with a WAV file
	if *flagTestFile != "" {
		log.Printf("Test mode: processing WAV file %s", *flagTestFile)

		// Start HTTP server in background
		http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry}))
		go func() {
			log.Printf("Starting HTTP server on %s", *flagAddr)
			log.Fatal(http.ListenAndServe(*flagAddr, nil))
		}()

		// Process the WAV file
		err := processWAVFile(*flagTestFile, m)
		if err != nil {
			log.Fatalf("Failed to process WAV file: %v", err)
		}

		// Keep the server running after processing
		log.Printf("WAV file processing complete. HTTP server still running on %s", *flagAddr)
		log.Printf("Press Ctrl+C to exit")
		select {} // Block forever
	}

	// Normal live audio processing mode
	log.Printf("Live audio mode: capturing from microphone")

	// Initialize A-weighting filter and time weighting
	log.Printf("Creating audio processing filters...")
	aFilter := newAWeightingFilter()
	slowWeighting := newTimeWeighting(1.0)   // 1 second for LAS
	fastWeighting := newTimeWeighting(0.125) // 0.125 second for LAF

	callbackCount := 0
	stream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, 0, func(in []float32) {
		callbackCount++
		processAudioChunk(in, m, aFilter, slowWeighting, fastWeighting, callbackCount)
	})

	if err != nil {
		log.Fatalf("Failed to open audio stream: %v", err)
	}
	log.Printf("Audio stream opened successfully")

	log.Printf("Starting audio stream...")
	err = stream.Start()
	if err != nil {
		log.Fatalf("Failed to start audio stream: %v", err)
	}
	defer stream.Close()
	log.Printf("Audio stream started successfully - now capturing audio")

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry}))
	log.Printf("Starting HTTP server on %s", *flagAddr)
	log.Fatal(http.ListenAndServe(*flagAddr, nil))
}
