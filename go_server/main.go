package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

func processRequest(totalLoopCount, base, exp float64) float64 {
	resultSum := 0.0
	for loopCount := 0.0; loopCount < totalLoopCount; loopCount++ {
		result := 0.0
		for i := math.Pow(base, exp); i >= 0; i-- {
			result += math.Atan(i) // * math.Tan(i)
		}
		resultSum += result
	}
	return resultSum
}

func convParamsToFloat(loopCount string, base string, exp string) (float64, float64, float64, bool) {
	loopCountFloat, err1 := strconv.ParseFloat(loopCount, 64)
	baseFloat, err2 := strconv.ParseFloat(base, 64)
	expFloat, err3 := strconv.ParseFloat(exp, 64)
	isErr := err1 != nil || err2 != nil || err3 != nil
	if isErr {
		fmt.Println(err1, ", ", err2, ", ", err3)
	}
	return loopCountFloat, baseFloat, expFloat, isErr
}

func getHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return err.Error()
	}
	return hostname
}

func respondWithError(w http.ResponseWriter, loopCount string, base string, exp string) {
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "Error at %s w/ loopCount=%s & compute=(%s,%s)", getHostName(), loopCount, base, exp)
}

func respondWithSuccess(w http.ResponseWriter, loopCount string, base string, exp string, reqResult float64) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "Processed at %s w/ loopCount=%s & compute=(%s,%s) => %f", getHostName(), loopCount, base, exp, reqResult)
}

func handleRequest(chIncrementNumOfReqs chan bool, w http.ResponseWriter, r *http.Request) {
	loopCount := r.URL.Query().Get("loopCount")
	base := r.URL.Query().Get("base")
	exp := r.URL.Query().Get("exp")

	loopCountFloat, baseFloat, expFloat, isErr := convParamsToFloat(loopCount, base, exp)
	if isErr {
		respondWithError(w, loopCount, base, exp)
		return
	}

	reqResult := processRequest(loopCountFloat, baseFloat, expFloat)

	respondWithSuccess(w, loopCount, base, exp, reqResult)
}

func getCentralControllerURL() string {
	ip := os.Getenv("CENTRAL_CONTROLLER_IP")
	port := 3000

	if ip == "" {
		ip = "10.101.101.101"
	}

	return fmt.Sprintf("http://%s:%d", ip, port)
}

func manageNumOfReqs(chIncrementNumOfReqs chan bool, chGetAndFlushNumOfReqs chan chan int) {

	numOfReqs := 0

	for {
		select {
		case <-chIncrementNumOfReqs:
			numOfReqs++
		case chReply := <-chGetAndFlushNumOfReqs:
			chReply <- numOfReqs
			numOfReqs = 0
		}
	}
}

type Response struct {
	ReqNum      int
	IsError     bool
	ErrMsg      string
	StatusCode  int
	Body        string
	StartTimeNs int64
	LatencyNs   int64
	ReadTimeNs  int64
}

func getWaitDuration(notifTimeIntervalNs time.Duration) time.Duration {

	currentTimeNs := time.Now().UnixNano()
	intervalNs := notifTimeIntervalNs.Nanoseconds()

	waitIntervalNs := intervalNs - (currentTimeNs % intervalNs)

	return time.Duration(waitIntervalNs)
}

// syncronous
func sendStateToCentralController(
	reqURL string,
	podname string,
	k int64,
	a int,
	tryNum int) Response {

	log.Printf("sending state [%s, %d, %d] to %s (try %d)", podname, k, a, reqURL, tryNum)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		errMsg := fmt.Sprintf("client: could not create request: %s", err)
		return Response{tryNum,
			true, errMsg,
			0, "",
			time.Now().UnixNano(), 0, 0}
	}
	req.Header.Set("Connection", "close")

	q := req.URL.Query()
	q.Add("podname", podname)
	q.Add("k", fmt.Sprintf("%d", k))
	q.Add("a", fmt.Sprintf("%d", a))
	req.URL.RawQuery = q.Encode()

	startReq := time.Now()
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}
	res, err := client.Do(req)
	latency := time.Since(startReq)

	if err != nil {
		errMsg := fmt.Sprintf("client: error making http request: %s", err)
		return Response{tryNum,
			true, errMsg,
			0, "",
			startReq.UnixNano(), latency.Nanoseconds(), 0}
	}

	startRead := time.Now()
	resBody, err := io.ReadAll(res.Body)
	readTime := time.Since(startRead)

	if err != nil {
		errMsg := fmt.Sprintf("client: could not read response body: %s", err)
		return Response{tryNum,
			true, errMsg,
			res.StatusCode, "",
			startReq.UnixNano(), latency.Nanoseconds(), readTime.Nanoseconds()}
	}

	return Response{tryNum,
		false, "",
		res.StatusCode, string(resBody),
		startReq.UnixNano(), latency.Nanoseconds(), readTime.Nanoseconds()}
}

func getAndFlushNumOfReqs(chGetAndFlushNumOfReqs chan chan int, chGetNumOfReqs chan int) int {
	chGetAndFlushNumOfReqs <- chGetNumOfReqs
	numOfReqs := <-chGetNumOfReqs
	return numOfReqs
}

// synchronous
func reliablySendState(chGetAndFlushNumOfReqs chan chan int, centralControllerURL string, chGetNumOfReqs chan int) {

	tryNum := 1
	podname, err := os.Hostname()
	if err != nil {
		log.Printf("Error: couldn't look up the hostname of pod\n")
	}
	numOfReqs := getAndFlushNumOfReqs(chGetAndFlushNumOfReqs, chGetNumOfReqs)
	currentTime := time.Now().UnixNano()

	for {
		resp := sendStateToCentralController(centralControllerURL, podname, currentTime, numOfReqs, tryNum)

		log.Printf("Resonse from CC for try %d: [%d] %s, {%s}, latency: %fms",
			tryNum, resp.StatusCode, resp.Body, resp.ErrMsg, float64(resp.LatencyNs)/1000000)

		if resp.StatusCode == 200 {
			break
		}

		if tryNum >= 3 {
			log.Printf("Error: no 200 response from CC in 3 tries. Stopping sending messages for k=%dns\n", currentTime)
			break
		}
		tryNum++
	}
}

func periodicallyNotifyCentralController(notifTimeInterval time.Duration, chGetAndFlushNumOfReqs chan chan int, centralControllerURL string) {

	// wait for a whole k interval of time
	waitDuration := getWaitDuration(notifTimeInterval)
	time.Sleep(waitDuration)

	// then after every k interval of time
	// reliably send state to the central controller
	repeatInterval := time.Duration(notifTimeInterval)
	repeatTicker := time.NewTicker(repeatInterval)
	chGetNumOfReqs := make(chan int)

	for range repeatTicker.C {
		reliablySendState(chGetAndFlushNumOfReqs, centralControllerURL, chGetNumOfReqs)
	}
}

func main() {

	portToListenOn := 3000
	chIncrementNumOfReqs := make(chan bool)
	chGetAndFlushNumOfReqs := make(chan chan int)
	centralControllerURL := getCentralControllerURL()
	notifTimeInterval := 1 * time.Second

	go manageNumOfReqs(chIncrementNumOfReqs, chGetAndFlushNumOfReqs)

	go periodicallyNotifyCentralController(notifTimeInterval, chGetAndFlushNumOfReqs, centralControllerURL)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(chIncrementNumOfReqs, w, r)
	})
	fmt.Printf("Server running (port=%d), route: http://localhost:%d/?loopCount=1&base=8&exp=7.7\n", portToListenOn, portToListenOn)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", portToListenOn), nil); err != nil {
		log.Fatal(err)
	}
}

/*
PROBLEMS:
-
*/
