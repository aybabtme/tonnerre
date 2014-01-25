package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	human "github.com/dustin/go-humanize"
)

var (
	concurrent int
	goalRps    int
	duration   time.Duration
	totalReq   int
	target     string

	listen  bool
	port    int
	respLen int
)

type Resp struct {
	dT     time.Duration
	code   int
	length uint64
	err    error
}

func parseArgs() {
	// Request mode
	flag.StringVar(&target, "target", "", "target to which requests will be sent")
	flag.IntVar(&concurrent, "concurrent", 10, "number of concurrent goroutine that will produce requests")
	flag.IntVar(&totalReq, "request", 1000, "total number of requests that will be produced")
	flag.IntVar(&goalRps, "rps", 1<<32, "throttle to that many requests per second")
	var durationStr string
	flag.StringVar(&durationStr, "duration", "", "duration of the stress test")

	// Listen mode
	flag.BoolVar(&listen, "listen", false, "run in listen mode, convenient to be the receiving end of tonnerre requests")
	flag.IntVar(&port, "port", 8080, "port on which to listen")
	flag.IntVar(&respLen, "response-len", 1024, "length of the response to return when in listen mode")
	flag.Parse()

	if len(target) == 0 && !listen {
		fmt.Fprintln(os.Stderr, "need at least the `target` flag, or to be in listen mode")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if len(durationStr) == 0 {
		duration = time.Hour * 1 << 16 // a very long time
	}
}

func main() {
	parseArgs()
	cores := runtime.NumCPU()
	n := runtime.GOMAXPROCS(cores)
	defer runtime.GOMAXPROCS(n)

	log.Printf("Will use %d cores\n", cores)

	if listen {
		startListen(cores)
		return
	}

	workers := sync.WaitGroup{}
	reqChan := make(chan int, 2*concurrent)
	respChan := make(chan Resp, 2*concurrent)

	log.Printf("Starting %d workers", concurrent)
	for worker := 0; worker < concurrent; worker++ {
		workers.Add(1)
		go requestWorker(&workers, reqChan, respChan, worker)
	}

	done := consumeResponses(respChan)

	go func() {
		req := 0
		// Fill task buffer every second
		for req < totalReq {
			select {
			case <-time.Tick(time.Second / 4):
				for j := 0; j < goalRps/4 && req < totalReq; j++ {
					req++
					reqChan <- req
				}
			}
		}
		close(reqChan)
		log.Printf("All %d requests queued, waiting for workers to finish.\n", totalReq)
	}()

	workers.Wait()
	log.Printf("All %d workers finished, waiting for responses to be consumed.\n", concurrent)
	close(respChan)
	<-done
	log.Println("All done.")
}

func requestWorker(wg *sync.WaitGroup, req <-chan int, resp chan<- Resp, workerID int) {
	log.Printf("Worker %d starting", workerID)
	defer wg.Done()
	buf := make([]byte, 8096)
	for reqID := range req {
		resp <- doRequest(reqID, buf)
	}
	log.Printf("Worker %d done", workerID)
}

func doRequest(reqID int, buf []byte) Resp {

	start := time.Now()
	resp, err := http.Get(target)

	r := Resp{
		dT:  time.Since(start),
		err: err,
	}

	if err == nil {
		r.code = resp.StatusCode
		r.length, r.err = countBytes(resp.Body, buf)
		resp.Body.Close()
	}

	return r
}

func countBytes(r io.Reader, buf []byte) (uint64, error) {
	var count uint64
	var n int
	var err error
	for err == nil {
		n, err = r.Read(buf)
		count += uint64(n)
	}

	if err != io.EOF {
		return count, err
	}

	return count, nil
}

func consumeResponses(resp <-chan Resp) <-chan struct{} {
	done := make(chan struct{})
	die := make(chan struct{})
	var byteCount uint64
	var completed uint64

	go func(die <-chan struct{}) {

		var byteC, completedC uint64
		var lastByteC, lastCompletedC uint64
		var byteDiff, reqDiff uint64

		tick := time.NewTicker(time.Second * 1)
		defer tick.Stop()

		start := time.Now()
		for {
			select {
			case <-die:
				fmt.Printf("\nDone in %v\n", time.Since(start))
				return
			case <-tick.C:

				byteC = atomic.LoadUint64(&byteCount)
				completedC = atomic.LoadUint64(&completed)

				byteDiff = byteC - lastByteC
				reqDiff = completedC - lastCompletedC
				fmt.Printf("\rTraffic: %5s\tRequest: %7d/%7dreqs\t%7s/s\t%7dreq/s",
					human.Bytes(byteC),
					completedC, totalReq,
					human.Bytes(byteDiff),
					reqDiff)

				lastByteC = byteC
				lastCompletedC = completedC
			}
		}
	}(die)

	go func(resp <-chan Resp, done chan<- struct{}) {

		codes := make(map[int]int)

		for r := range resp {
			codes[r.code]++

			atomic.AddUint64(&completed, 1)
			atomic.AddUint64(&byteCount, r.length)
		}

		log.Printf("All %d responses received. Results:\n", completed)
		for key, val := range codes {
			log.Printf("\tcode=%d, occurences=%d\n", key, val)
		}
		die <- struct{}{}
		done <- struct{}{}
	}(resp, done)

	return done
}
