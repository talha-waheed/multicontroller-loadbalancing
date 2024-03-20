package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func forwardReq(
	w http.ResponseWriter,
	req *http.Request,
	lb *LoadBalancer,
	reqNum int) {

	// we need to buffer the body if we want to read it here and send it
	// in the request.
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loopCount := req.URL.Query().Get("loopCount")
	base := req.URL.Query().Get("base")
	exp := req.URL.Query().Get("exp")

	// you can reassign the body if you need to parse it as multipart
	req.Body = io.NopCloser(bytes.NewReader(body))

	endpoint := lb.GetEndpointForReq(reqNum)

	// create a new url from the raw RequestURI sent by the client
	url := fmt.Sprintf("http://%s:3000/?loopCount=%s&base=%s&exp=%s",
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

func getEndpoints() []Endpoint {
	ipsConcatenated := os.Getenv("IPS")
	nodesConcatenated := os.Getenv("NODES")
	app := os.Getenv("APP")

	fmt.Println(ipsConcatenated, nodesConcatenated, app)

	ips := strings.Split(ipsConcatenated, ",")

	nodes := strings.Split(nodesConcatenated, ",")
	nodesInt := make([]int, 0)
	for _, node := range nodes {
		nodeInt, err := strconv.Atoi(node)
		check(err)
		nodesInt = append(nodesInt, nodeInt)
	}

	appInt, err := strconv.Atoi(app)
	check(err)

	endpoints := make([]Endpoint, 0)
	for i, ip := range ips {
		endpoints = append(endpoints, Endpoint{
			URL:  ip,
			Node: nodesInt[i],
			App:  appInt,
		})
	}

	return endpoints
}

func main() {

	portToListenOn := 3000

	endpoints := getEndpoints()

	// print endpoints
	fmt.Println("Endpoints:", endpoints)

	// set load balancer algorithm
	loadBalancerAlgo := "NONE"
	if len(endpoints) > 1 {
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
	})
	fmt.Printf("Server running (port=%d), route: http://localhost:%d/?loopCount=1&base=8&exp=7.7\n", portToListenOn, portToListenOn)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", portToListenOn), nil); err != nil {
		log.Fatal(err)
	}
}
