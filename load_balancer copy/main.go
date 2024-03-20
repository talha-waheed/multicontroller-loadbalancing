package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func forwardReq(
	w http.ResponseWriter,
	req *http.Request,
	lb *LoadBalancer,
	reqNum int) {

	// we need to buffer the body if we want to read it here and send it
	// in the request.
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loopCount := req.URL.Query().Get("loopCount")
	base := req.URL.Query().Get("base")
	exp := req.URL.Query().Get("exp")

	// you can reassign the body if you need to parse it as multipart
	req.Body = ioutil.NopCloser(bytes.NewReader(body))

	endpoint := lb.GetEndpointForReq(reqNum)

	// create a new url from the raw RequestURI sent by the client
	url := fmt.Sprintf("http://%s/?loopCount=%s&base=%s&exp=%s",
		endpoint.URL, loopCount, base, exp)

	proxyReq, err := http.NewRequest(req.Method, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// We may want to filter some headers, otherwise we could just use a shallow copy
	// proxyReq.Header = req.Header
	proxyReq.Header = make(http.Header)
	for h, val := range req.Header {
		proxyReq.Header[h] = val
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	resBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Connection", "close")
	fmt.Fprint(w, string(resBody))

	lb.NotifyReqCompleted(reqNum)
}

type Endpoint struct {
	URL  string `json:"url"`
	Node int    `json:"node"`
	App  int    `json:"app"`
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	portToListenOn := 3500

	// chIncrementNumOfReqs := make(chan bool)
	// chGetAndFlushNumOfReqs := make(chan chan int)
	// centralControllerURL := getCentralControllerURL()
	// notifTimeInterval := 1 * time.Second
	// go manageNumOfReqs(chIncrementNumOfReqs, chGetAndFlushNumOfReqs)
	// go periodicallyNotifyCentralController(notifTimeInterval, chGetAndFlushNumOfReqs, centralControllerURL)

	// rds := RedisClient{client: getRedisClient()}

	endpoints := []Endpoint{
		{URL: "localhost:3000", Node: 1, App: 1},
		{URL: "localhost:3001", Node: 1, App: 2},
	}
	reqURLs := strings.Split("localhost:3000,localhost:3001", ",")

	// set load balancer algorithm
	loadBalancerAlgo := "NONE"
	if len(reqURLs) > 1 {
		loadBalancerAlgo = "LEAST_REQUEST"
	}
	fmt.Printf("Load balancer algorithm: %s\n", loadBalancerAlgo)

	// start the load balancer
	lb := LoadBalancer{
		loadBalancerAlgo: loadBalancerAlgo,
		endpoints:        endpoints,
	}
	lb.StartLoadBalancer()

	reqNum := 0

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		forwardReq(w, r, &lb, reqNum)
		// reqNum++
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
