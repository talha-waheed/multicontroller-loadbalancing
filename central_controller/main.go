package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strconv"
)

type Req struct {
	podname string
	k       int
	a       int
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

type Host struct {
	name         string
	ipaddress    string
	loadcapacity int
	podnames     []string
}

type Pod struct {
	name      string
	ipaddress string
	hostname  string
	LBname    string
}

type PodProps struct {
	ipaddress string
	hostname  string
	LBname    string
}

type LBProps struct {
	podnames []string
}

func getInitHostPrices(hosts []Host) map[string]float64 {

	initPrice := 1.0

	initHostPrices := make(map[string]float64)

	for i := 0; i < len(hosts); i++ {
		initHostPrices[hosts[i].name] = initPrice
	}

	return initHostPrices
}

func getInitPodLoads(pods []Pod) map[string]int {

	initReqsReceived := -1

	initPodLoads := make(map[string]int)

	for i := 0; i < len(pods); i++ {
		initPodLoads[pods[i].name] = initReqsReceived
	}

	return initPodLoads
}

func getAllPodLoads(pods []Pod, chListenReqs chan Req) map[string]int {

	podLoads := getInitPodLoads(pods)

	uniquePodLoadsReceived := 0

	for {

		// listen for requests
		req := <-chListenReqs

		// ignore this if its a pod that we don't recognize
		_, ok := podLoads[req.podname]
		if !ok {
			fmt.Printf("Unrecognized pod sent request: [%s, k=%d, a=%d]. Request ignored", req.podname, req.k, req.a)
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
	hosts []Host,
	podLoads map[string]int,
) map[string]int {

	hostLoads := make(map[string]int)

	for i := 0; i < len(hosts); i++ {
		hostLoads[hosts[i].name] = 0
		for j := 0; j < len(hosts[i].podnames); j++ {
			podname := hosts[i].podnames[j]
			hostLoads[hosts[i].name] += podLoads[podname]
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
	// Srikanth's Algorithm is implemented in this function to calculate the new host prices

	newPrice := math.Abs(
		oldPrice +
			epsilon*(float64(loadArrivedAtHost)-
				float64(hostLoadCapacity)+
				(1/sumOfOldHostPrices)))

	return newPrice
}

func getNewHostPrices(
	pods []Pod,
	hosts []Host,
	podLoads map[string]int,
	oldHostPrices map[string]float64,
) map[string]float64 {

	loadsArrivedAtHost := aggregatePodLoadstoHostLoads(hosts, podLoads)

	newHostPrices := make(map[string]float64)

	sumOfOldHostPrices := getSumOfPrices(oldHostPrices)

	for i := 0; i < len(hosts); i++ {
		hostname := hosts[i].name
		newHostPrices[hostname] = getNewHostPrice(
			oldHostPrices[hostname],
			1.0,
			loadsArrivedAtHost[hostname],
			hosts[i].loadcapacity,
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
		optimalHost := getLeastPricedHost(lb.podnames, pods, hostprices)
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
		hostname := pods[podname].hostname
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

func centralController(
	hosts []Host,
	pods []Pod,
	LBs map[string]LBProps,
	podHost map[string]string,
	chListenReqs chan Req) {

	// define state at the beginning of the controller
	hostPrices := getInitHostPrices(hosts)
	podsMap := getPodsPropsMapped(pods)

	for {

		// wait for each pod to send state (# of reqs it received in time k)
		podLoads := getAllPodLoads(pods, chListenReqs)

		// compute price for each host
		hostPrices = getNewHostPrices(pods, hosts, podLoads, hostPrices)

		// determine what is the optimal hostname for each LB (according to lowest host price)
		optimalHostsForLBs := getOptimalHostsForLBs(LBs, podsMap, hostPrices)

		// communicate optimal hostname to each LB
		communicateOptimalHostsToLBs(LBs, optimalHostsForLBs)

		// compute theta for next hosts (implicitly done on the next iteration)

	}

}

func communicateOptimalHostsToLBs(LBs map[string]LBProps, optimalHostsForLBs map[string]string) {
	panic("unimplemented")
	// TO-DO:
	// for each LB
	// 		make an async request to its IP:port
	// 			telling it the IP:port of its optimal host
}

func getPodsPropsMapped(pods []Pod) map[string]PodProps {

	podMap := make(map[string]PodProps)
	for i := 0; i < len(pods); i++ {
		podMap[pods[i].name] = PodProps{pods[i].ipaddress, pods[i].hostname, pods[i].LBname}
	}
	return podMap
}

func main() {

	chListenReqs := make(chan Req)

	/* start a thread that will process all the price updates coming
	*  from the hosts
	 */
	go centralController(chListenReqs)

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
*	- fixed topology
*	- we have to manually figure our the topology
*	- we ignore failures
*	- we are not ensuring same k is used for calculations
*	- we should change Host, Pod, LB to maps of [hostname]HostProps,[podname]PodProps, [LBname]LBProps
*	- Maybe breaking ties strategy of mine is wasting compute
 */
