package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

type Req struct {
	podname string
	k       int
	a       int
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

type HostProps struct {
	Name         string   `json:"name"`
	LoadCapacity int      `json:"loadCapacity"`
	PodNames     []string `json:"podNames"`
}

type PodProps struct {
	Name      string `json:"name"`
	IPAddress string `json:"ipAddress"`
	HostName  string `json:"hostName"`
	LBname    string `json:"lbName"`
}

type LBProps struct {
	Name      string   `json:"name"`
	IPAddress string   `json:"ipAddress"`
	PodNames  []string `json:"podNames"`
}

func respondWithError(w http.ResponseWriter, errStr string) {
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "%s", errStr)
}

func respondWithSuccess(w http.ResponseWriter, req Req) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "Enqueued req for processing [for %s w/ k=%d & a=%d]", req.podname, req.k, req.a)
}

func getQueryParams(r *http.Request) (string, int, int, error) {
	podname := r.URL.Query().Get("podname")
	kStr := r.URL.Query().Get("k")
	k, err := strconv.Atoi(kStr)
	if err != nil {
		return "", 0, 0, err
	}
	aStr := r.URL.Query().Get("a")
	a, err2 := strconv.Atoi(aStr)
	if err2 != nil {
		return "", 0, 0, err2
	}
	return podname, k, a, nil
}

func handleRequest(chListenReqs chan Req, w http.ResponseWriter, r *http.Request) {
	podname, k, a, err := getQueryParams(r)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, fmt.Sprintf("%s", err))
		return
	}

	req := Req{podname, k, a}

	// send request for processing in central controller
	chListenReqs <- req

	respondWithSuccess(w, req)
}

func getInitHostPrices(hosts map[string]HostProps) map[string]float64 {

	initPrice := 1.0

	initHostPrices := make(map[string]float64)

	for hostname := range hosts {
		initHostPrices[hostname] = initPrice
	}

	return initHostPrices
}

func getInitPodLoads(pods map[string]PodProps) map[string]int {

	initReqsReceived := -1

	initPodLoads := make(map[string]int)

	for podname := range pods {
		initPodLoads[podname] = initReqsReceived
	}

	return initPodLoads
}

// function to repeatedly fo something every 1s
func doEvery(d time.Duration, f func(time.Time)) {
	for x := range time.Tick(d) {
		f(x)
	}
}

func getHostLoads(
	hosts map[string]HostProps,
	redisClients map[string]*redis.Client) map[string]int {

	// get all host loads from all hosts
	hostLoads := make(map[string]int)

	for hostName, client := range redisClients {

		hostLoad, err := client.Get(ctx, "outstanding_requests").Result()
		if err == redis.Nil {
			log.Println("outstanding_requests does not exist")
			hostLoad = "0"
		} else if err != nil {
			hostLoad = "0"
			log.Printf("Error: couldn't get variable from Redis\n")
		}

		log.Printf("%s: outstanding_requests = %s\n", hostName, hostLoad)

		hostLoadInt, err2 := strconv.Atoi(hostLoad)
		if err2 != nil {
			log.Printf("Error: couldn't convert outstanding_requests (%s) to int\n", hostLoad)
			continue
		}
		hostLoads[hostName] = hostLoadInt
	}

	return hostLoads
}

func getAllPodLoads(pods map[string]PodProps, chListenReqs chan Req) map[string]int {

	podLoads := getInitPodLoads(pods)

	uniquePodLoadsReceived := 0

	for {

		// listen for requests
		req := <-chListenReqs

		// ignore this if its a pod that we don't recognize
		_, ok := podLoads[req.podname]
		if !ok {
			fmt.Printf("Unrecognized pod sent request: [%s, k=%d, a=%d]. Request ignored", req.podname, req.k, req.a)
			continue
		}

		// check if the sender is a pod we have not heard from before in this loop
		if podLoads[req.podname] == -1 {
			uniquePodLoadsReceived++
		}

		// update the state
		podLoads[req.podname] = req.a

		// if we have listened from all pods, break from loop
		if uniquePodLoadsReceived == len(pods) {
			break
		}
	}

	return podLoads
}

func aggregatePodLoadstoHostLoads(
	hosts map[string]HostProps,
	podLoads map[string]int,
) map[string]int {

	hostLoads := make(map[string]int)

	for hostname, hostprops := range hosts {
		hostLoads[hostname] = 0
		for j := 0; j < len(hostprops.PodNames); j++ {
			podname := hostprops.PodNames[j]
			hostLoads[hostname] += podLoads[podname]
		}
	}

	return hostLoads
}

func getNewHostPrice(
	oldPrice float64,
	epsilon float64,
	loadArrivedAtHost int,
	hostLoadCapacity int,
	sumOfOldHostPrices float64,
) float64 {
	// Prof. Srikanth's Algorithm is implemented in this function to calculate the new host prices

	newPrice := math.Abs(
		oldPrice +
			epsilon*(float64(loadArrivedAtHost)-
				float64(hostLoadCapacity)+
				(1/sumOfOldHostPrices)))

	return newPrice
}

func getNewHostPrices(
	pods map[string]PodProps,
	hosts map[string]HostProps,
	hostLoads map[string]int,
	oldHostPrices map[string]float64,
) map[string]float64 {

	loadsArrivedAtHost := hostLoads

	newHostPrices := make(map[string]float64)

	sumOfOldHostPrices := getSumOfPrices(oldHostPrices)

	for hostname, hostprops := range hosts {
		newHostPrices[hostname] = getNewHostPrice(
			oldHostPrices[hostname],
			1.0,
			loadsArrivedAtHost[hostname],
			hostprops.LoadCapacity,
			sumOfOldHostPrices,
		)
	}

	return newHostPrices
}

func getSumOfPrices(oldHostPrices map[string]float64) float64 {
	sum := 0.0
	for _, v := range oldHostPrices {
		sum += v
	}
	return sum
}

func getOptimalHostsForLBs(LBs map[string]LBProps, pods map[string]PodProps, hostprices map[string]float64) map[string]string {

	optimalHosts := make(map[string]string)

	// get optimal for each LB
	for lbName, lb := range LBs {
		optimalHost := getLeastPricedHost(lb.PodNames, pods, hostprices)
		optimalHosts[lbName] = optimalHost
	}

	return optimalHosts
}

func getLeastPricedHost(podnames []string, pods map[string]PodProps, hostprices map[string]float64) string {

	// shuffle podnames so that we can break ties randomly
	podnames = getShuffledArray(podnames)

	minPrice := math.MaxFloat64
	minHost := ""

	for _, podname := range podnames {
		hostname := pods[podname].HostName
		hostprice := hostprices[hostname]
		if hostprice <= minPrice {
			minPrice = hostprice
			minHost = hostname
		}
	}

	return minHost
}

func getShuffledArray(arr []string) []string {
	for i := range arr {
		j := rand.Intn(i + 1)
		arr[i], arr[j] = arr[j], arr[i]
	}
	return arr
}

// syncronous
func makeRequest(reqURL string, podIP string, reqNum int) Response {

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		errMsg := fmt.Sprintf("client: could not create request: %s", err)
		return Response{reqNum,
			true, errMsg,
			0, "",
			time.Now().UnixNano(), 0, 0}
	}
	req.Header.Set("Connection", "close")

	q := req.URL.Query()
	q.Add("endpoints", podIP)
	req.URL.RawQuery = q.Encode()

	startReq := time.Now()
	res, err := http.DefaultClient.Do(req)
	latency := time.Since(startReq)

	if err != nil {
		errMsg := fmt.Sprintf("client: error making http request: %s", err)
		return Response{reqNum,
			true, errMsg,
			0, "",
			startReq.UnixNano(), latency.Nanoseconds(), 0}
	}

	startRead := time.Now()
	resBody, err := io.ReadAll(res.Body)
	readTime := time.Since(startRead)

	if err != nil {
		errMsg := fmt.Sprintf("client: could not read response body: %s", err)
		return Response{reqNum,
			true, errMsg,
			res.StatusCode, "",
			startReq.UnixNano(), latency.Nanoseconds(), readTime.Nanoseconds()}
	}

	return Response{reqNum,
		false, "",
		res.StatusCode, string(resBody),
		startReq.UnixNano(), latency.Nanoseconds(), readTime.Nanoseconds()}
}

func getPodIPonGivenHost(
	optimalHostName string,
	LB LBProps,
	pods map[string]PodProps) string {

	for _, podName := range LB.PodNames {
		if pods[podName].HostName == optimalHostName {
			return pods[podName].IPAddress

		}
	}

	return ""
}

// syncronous
func communicateOptimalPodIPToLB(
	optimalHostName string,
	LB LBProps,
	pods map[string]PodProps,
	chNotifyReqCompleted chan bool) {

	log.Printf("LB Update: %s -> %s\n", LB.Name, optimalHostName)

	lbUrl := fmt.Sprintf("http://%s", LB.IPAddress)
	optimalPodIP := getPodIPonGivenHost(optimalHostName, LB, pods)
	reqNum := 1

	res := makeRequest(lbUrl, optimalPodIP, reqNum)
	log.Printf("Response sent to %s\n", LB.Name)

	log.Printf("Response received from %s: %d\n", LB.Name, res.StatusCode)

	chNotifyReqCompleted <- true
}

func communicateOptimalHostsToLBs(
	LBs map[string]LBProps,
	optimalHostsForLBs map[string]string,
	pods map[string]PodProps) {
	// TO-DO:
	// for each LB
	// 		make an async request to its IP:port
	// 			telling it the IP:port of its optimal host

	numReqsCompleted := 0
	chNotifyReqCompleted := make(chan bool)

	for LBname, LBProps := range LBs {
		go communicateOptimalPodIPToLB(optimalHostsForLBs[LBname], LBProps, pods, chNotifyReqCompleted)
	}

	for {
		<-chNotifyReqCompleted
		numReqsCompleted++

		if numReqsCompleted == len(LBs) {
			break
		}
	}

	log.Printf("LB Update: --------COMPLETED--------\n")
}

func centralController(
	hosts map[string]HostProps,
	pods map[string]PodProps,
	LBs map[string]LBProps,
	interval time.Duration,
	chListenReqs chan Req,
	redisClients map[string]*redis.Client) {

	// define state at the beginning of the controller
	hostPrices := getInitHostPrices(hosts)

	for t := range time.Tick(interval) {

		// print the current time
		log.Printf("CC logic starting [time: %s]\n", t)

		// wait for each pod to send state (# of reqs it received in time k)
		hostLoads := getHostLoads(hosts, redisClients)

		// compute price for each host
		hostPrices = getNewHostPrices(pods, hosts, hostLoads, hostPrices)

		// determine what is the optimal hostname for each LB (according to lowest host price)
		optimalHostsForLBs := getOptimalHostsForLBs(LBs, pods, hostPrices)

		// communicate optimal hostname to each LB
		communicateOptimalHostsToLBs(LBs, optimalHostsForLBs, pods)

		// compute theta for next hosts
		// (no need to do this here. It is implicitly done in calculating new host prices)
	}
}

func getHostsListMappedToName(hostsList []HostProps) map[string]HostProps {
	hostsMap := make(map[string]HostProps)
	for _, hostProps := range hostsList {
		hostsMap[hostProps.Name] = hostProps
	}
	return hostsMap
}

func getPodsListMappedToName(podsList []PodProps) map[string]PodProps {
	podsMap := make(map[string]PodProps)
	for _, podProps := range podsList {
		podsMap[podProps.Name] = podProps
	}
	return podsMap
}

func getLBsListMappedToName(lbsList []LBProps) map[string]LBProps {
	lbsMap := make(map[string]LBProps)
	for _, lbProps := range lbsList {
		lbsMap[lbProps.Name] = lbProps
	}
	return lbsMap
}

func getTopology() (map[string]HostProps, map[string]PodProps, map[string]LBProps) {

	hostsJSON := os.Getenv("HOSTS")
	var hostsList []HostProps
	json.Unmarshal([]byte(hostsJSON), &hostsList)
	hostsMap := getHostsListMappedToName(hostsList)

	podsJSON := os.Getenv("PODS")
	var podsList []PodProps
	json.Unmarshal([]byte(podsJSON), &podsList)
	podsMap := getPodsListMappedToName(podsList)

	lbsJSON := os.Getenv("LBS")
	var lbsList []LBProps
	json.Unmarshal([]byte(lbsJSON), &lbsList)
	lbsMap := getLBsListMappedToName(lbsList)

	return hostsMap, podsMap, lbsMap
}

func logTopology(
	hosts map[string]HostProps,
	pods map[string]PodProps,
	LBs map[string]LBProps) {

	log.Println("hosts: ", hosts)
	log.Println("pods: ", pods)
	log.Println("LBs: ", LBs)

}

func getRedisIP(hostName string) string {
	if hostName == "minikube-m02" {
		return "10.101.102.101"
	} else if hostName == "minikube-m03" {
		return "10.101.102.102"
	} else if hostName == "minikube-m04" {
		return "10.101.102.103"
	} else {
		return "localhost"
	}
}

func getRedisClientsForHosts(hosts map[string]HostProps) map[string]*redis.Client {
	redisClients := make(map[string]*redis.Client)

	for _, hostProps := range hosts {
		redisClients[hostProps.Name] = redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:6379", getRedisIP(hostProps.Name)),
			Password: "",
			DB:       0,
		})
	}

	return redisClients
}

func getInterval() time.Duration {
	intervalMs, err := strconv.Atoi(os.Getenv("INTERVAL_MS"))
	if err != nil {
		log.Fatal(err)
	}
	return time.Duration(intervalMs) * time.Millisecond
}

func main() {

	hosts, pods, LBs := getTopology()
	logTopology(hosts, pods, LBs)

	chListenReqs := make(chan Req)

	redisClients := getRedisClientsForHosts(hosts)

	interval := getInterval()

	/* start a thread that will process all the price updates coming
	*  from the hosts
	 */
	go centralController(hosts, pods, LBs, interval, chListenReqs, redisClients)

	port := 3000

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(chListenReqs, w, r)
	})
	fmt.Printf("Server running (port=%d), listening for # of requests from pods [http://localhost:%d/?podname=1&a=5]\n", port, port)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}

/* PROBLEMS:
*	- We have a fixed topology
*	- We have to manually figure our the topology
*	- We ignore failures
*	- We are not ensuring same k is used for calculations
*	- We should change Host, Pod, LB to maps of [hostname]HostProps,[podname]PodProps, [LBname]LBProps
*	- There can be race conditions in the system between requests from pods to controller
*	- Maybe breaking ties strategy of mine is wasting compute
*	- Notifiying the LBs their new optimal host is done unreliably (maybe this is better to do)
 */
