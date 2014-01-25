package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

var (
	concurrent   int
	goalRps      int
	testDuration time.Duration
	totalReq     int
	target       string

	listen  bool
	port    int
	respLen int
)

func parseArgs() {
	defaultDuration := time.Hour * (1 << 16)
	// Request mode
	flag.StringVar(&target, "target", "", "target to which requests will be sent")
	flag.IntVar(&concurrent, "concurrent", 10, "number of concurrent goroutine that will produce requests")
	flag.IntVar(&totalReq, "request", -1, "total number of requests that will be produced")
	flag.IntVar(&goalRps, "rps", 1000000, "throttle to that many requests per second")
	flag.DurationVar(&testDuration, "duration", defaultDuration, "duration of the stress test")

	// Listen mode
	flag.BoolVar(&listen, "listen", false, "run in listen mode, convenient to be the receiving end of tonnerre requests")
	flag.IntVar(&port, "port", 8080, "port on which to listen")
	flag.IntVar(&respLen, "response-len", 1024, "length of the response to return when in listen mode")
	flag.Parse()

	if listen {
		return
	}

	// Assert the flags are sufficient to work
	if len(target) == 0 {
		fmt.Fprintln(os.Stderr, "need at least the `target` flag, or to be in listen mode")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if testDuration == defaultDuration && totalReq == -1 {
		fmt.Fprintln(os.Stderr, "need to specify at least a `duration` or a number of `request`")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if testDuration != defaultDuration {
		log.Printf("Will run test for %v", testDuration)
	}
}
