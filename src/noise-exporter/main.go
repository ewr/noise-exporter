package main

import (
	"flag"
	"log"
	"math"
	"time"

	"github.com/gordonklaus/portaudio"
)

var (
	flagAddr  = flag.String("addr", "", "Listen on address")
	flagDebug = flag.Bool("debug", false, "Print debug messages")
)

const sampleRate = 48000
const channels = 1
const bufferSeconds = 30
const pRef = 20e-6

func main() {
	flag.Parse()

	portaudio.Initialize()
	defer portaudio.Terminate()

	// buffer := make([]float32, sampleRate*bufferSeconds)

	stream, err := portaudio.OpenDefaultStream(channels, 0, sampleRate, 0, func(in []float32) {
		// log.Printf("Buffer callback %x", in)

		decibels := make([]float64, len(in))
		sum := 0
		for i := range in {
			decibels[i] = 20 * math.Log10(math.Abs(float64(in[i]))/pRef)
			sum += int(decibels[i])
		}

		log.Printf("db avg is %d", sum/len(in))
	})

	if err != nil {
		panic(err)
	}

	stream.Start()
	defer stream.Close()

	log.Printf("at end")

	for {
		time.Sleep(1 * time.Second)
	}
}
