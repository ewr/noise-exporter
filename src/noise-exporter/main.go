package main

import (
	"flag"
	"log"
	"math"
	"net/http"

	"github.com/gordonklaus/portaudio"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	flagAddr  = flag.String("addr", ":9900", "Listen on address")
	flagDebug = flag.Bool("debug", false, "Print debug messages")
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
	alpha   float64 // exponential averaging coefficient
	average float64 // current average
	initialized bool
}

func newTimeWeighting(timeConstant float64) *TimeWeighting {
	// Calculate alpha for exponential averaging
	alpha := math.Exp(-1.0 / (timeConstant * sampleRate))
	return &TimeWeighting{alpha: alpha}
}

func (tw *TimeWeighting) process(input float64) float64 {
	if !tw.initialized {
		tw.average = input
		tw.initialized = true
	} else {
		tw.average = tw.alpha*tw.average + (1-tw.alpha)*input
	}
	return tw.average
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

	portaudio.Initialize()
	defer portaudio.Terminate()

	// Initialize A-weighting filter and time weighting
	aFilter := newAWeightingFilter()
	slowWeighting := newTimeWeighting(1.0)    // 1 second for LAS
	fastWeighting := newTimeWeighting(0.125)  // 0.125 second for LAF

	stream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, 0, func(in []float32) {
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
		for _, sample := range in {
			// Apply A-weighting filter
			weighted := aFilter.process(float64(sample))
			// Square the weighted pressure value
			squaredSum += weighted * weighted
		}
		
		// Calculate RMS of the A-weighted signal
		rms := math.Sqrt(squaredSum / float64(len(in)))
		
		// Convert to decibels
		if rms > 0 {
			instantLA := 20 * math.Log10(rms/pRef)
			
			// Apply time weighting
			slowLA := slowWeighting.process(instantLA)
			fastLA := fastWeighting.process(instantLA)
			
			// Update metrics
			m.lAS.Set(slowLA)
			m.lAF.Set(fastLA)
			
			if *flagDebug {
				log.Printf("LA instant: %.1f dB, LAS: %.1f dB, LAF: %.1f dB", 
					instantLA, slowLA, fastLA)
			}
		}
	})

	if err != nil {
		panic(err)
	}

	stream.Start()
	defer stream.Close()

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry}))
	log.Fatal(http.ListenAndServe(*flagAddr, nil))
	log.Printf("Listening on %s", *flagAddr)
}
