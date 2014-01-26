package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	human "github.com/dustin/go-humanize"
)

const (
	rateResolution int = 25 //'th of a second
)

type Resp struct {
	dT     time.Duration
	code   int
	length uint64
	err    error
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

	workWG := sync.WaitGroup{}
	reqChan := make(chan int, 2*concurrent)
	respChan := make(chan Resp, 2*concurrent)

	log.Printf("Starting %d workers", concurrent)
	for worker := 0; worker < concurrent; worker++ {
		workWG.Add(1)
		go startRequestWorker(&workWG, reqChan, respChan, worker)
	}
	go sendTaskToWorkers(reqChan)

	done := consumeResponses(respChan)

	workWG.Wait()
	close(respChan)
	<-done
	log.Println("All done.")
}

func startRequestWorker(wg *sync.WaitGroup, req <-chan int, resp chan<- Resp, workerID int) {
	defer wg.Done()
	for reqID := range req {
		resp <- doRequest(reqID)
	}
}

func doRequest(reqID int) Resp {

	start := time.Now()
	resp, err := http.Get(target)

	r := Resp{
		dT:  time.Since(start),
		err: err,
	}

	if err == nil {
		r.code = resp.StatusCode
		r.length, r.err = countBytes(resp.Body)
		resp.Body.Close()
	}

	return r
}

func countBytes(r io.Reader) (uint64, error) {
	count, err := io.Copy(ioutil.Discard, r)
	return uint64(count), err
}

func sendTaskToWorkers(reqChan chan<- int) {
	defer close(reqChan)

	exhaustion := time.NewTicker(testDuration)
	rateLimiter := time.NewTimer(time.Second / time.Duration(rateResolution))

	for req := 0; req < totalReq; {

		select {
		case <-rateLimiter.C:
			for j := 0; j < goalRps/rateResolution && req < totalReq; j++ {
				req++
				reqChan <- req
			}
		case <-exhaustion.C:
			return
		}
	}
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

		die <- struct{}{}
		for key, val := range codes {
			log.Printf("\tcode=%d, occurences=%d\n", key, val)
		}
		done <- struct{}{}
	}(resp, done)

	return done
}
