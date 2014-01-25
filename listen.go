package main

import (
	"bytes"
	"fmt"
	"github.com/aybabtme/gypsum"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/aybabtme/humanize"
)

func startListen(concurrency int) {

	// 2* to give some buffering in case reader is slower than writer
	inChan := make(chan int, 2*concurrency)
	outChan := make(chan int, 2*concurrency)
	errC := make(chan error, 2*concurrency)

	http.HandleFunc("/", createHandlerFunc(inChan, outChan, errC))

	go printStatsfunc(inChan, outChan, errC)

	localAddr := net.JoinHostPort("", strconv.Itoa(port))
	log.Printf("Listening: %s", localAddr)
	http.ListenAndServe(localAddr, nil)
}

func createHandlerFunc(hitChan chan<- int, backChan chan<- int, errChan chan<- error) http.HandlerFunc {

	buf := prepareRespBuffer(respLen)

	return func(rw http.ResponseWriter, req *http.Request) {
		reqBuf := bytes.NewBuffer(nil)
		err := req.Header.Write(reqBuf)
		if err != nil {
			errChan <- fmt.Errorf("reading request header, %v", err)
		}

		_, err = io.Copy(reqBuf, req.Body)
		if err != nil {
			errChan <- fmt.Errorf("reading request body, %v", err)
		}
		hitChan <- int(reqBuf.Len())

		err = req.Body.Close()
		if err != nil {
			errChan <- fmt.Errorf("closing request body, %v", err)
		}
		m, err := rw.Write(buf)
		if err != nil {
			errChan <- fmt.Errorf("writing back answer, %v", err)
		}
		backChan <- m
	}
}

func prepareRespBuffer(length int) []byte {

	buf := bytes.NewBuffer(make([]byte, 0, length))

	buf.WriteString("<!doctype html><body>")

	for {
		miss := length - buf.Len()
		lorem := gypsum.ArticleLorem(100, "<br/>")
		if len(lorem) < miss {
			buf.WriteString(lorem)
		} else {
			buf.WriteString(lorem[:miss])
			break
		}
	}

	return buf.Bytes()
}

func printStatsfunc(inChan <-chan int, outChan <-chan int, errChan <-chan error) {

	var (
		requestIn     int
		lastRequestIn int
		diffReqIn     int

		requestOut     int
		lastRequestOut int
		diffReqOut     int

		trafficIn     int
		lastTrafficIn int
		diffIn        int

		trafficOut     int
		lastTrafficOut int
		diffOut        int
	)
	var in, out int
	var err error
	tick := time.NewTicker(time.Second * 1)

	for {
		select {
		case in = <-inChan:
			trafficIn += in
			requestIn++
		case out = <-outChan:
			trafficOut += out
			requestOut++
		case err = <-errChan:
			log.Printf("error: %v\n", err)
		case <-tick.C:
			diffReqIn = requestIn - lastRequestIn
			diffReqOut = requestOut - lastRequestOut
			diffIn = trafficIn - lastTrafficIn
			diffOut = trafficOut - lastTrafficOut

			fmt.Printf("\r"+
				"%8dreq, %4s, %8srps, %4sps"+
				"  ---> "+
				"%8dreq, %4s, %8srps, %4sps",
				requestIn,
				humanize.Bytes(uint64(trafficIn)),
				humanize.Comma(int64(diffReqIn)),
				humanize.Bytes(uint64(diffIn)),
				requestOut,
				humanize.Bytes(uint64(trafficOut)),
				humanize.Comma(int64(diffReqOut)),
				humanize.Bytes(uint64(diffOut)))

			lastTrafficIn = trafficIn
			lastTrafficOut = trafficOut
			lastRequestIn = requestIn
			lastRequestOut = requestOut
		}
	}
}
