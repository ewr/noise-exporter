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
}

const sampleRate = 48000
const channels = 1
const bufferSeconds = 30
const pRef = 20e-6

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		lAeq: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "noise_exporter_loudness_instant",
			Help: "Latest loudness reading (LAeq)",
		}),
	}
	reg.MustRegister(m.lAeq)
	return m
}

func main() {
	flag.Parse()

	registry := prometheus.NewRegistry()
	m := newMetrics(registry)

	portaudio.Initialize()
	defer portaudio.Terminate()

	// buffer := make([]float32, sampleRate*bufferSeconds)

	// lastDbAvg := 0

	stream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, 0, func(in []float32) {
		decibels := make([]float64, len(in))
		var sum float64 = 0
		for i := range in {
			decibels[i] = 20 * math.Log10(math.Abs(float64(in[i]))/pRef)
			sum += float64(decibels[i])
		}

		// log.Printf("db avg is %d", sum/len(in))
		// lastDbAvg = sum / len(in)
		m.lAeq.Set(sum / float64(len(in)))
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
