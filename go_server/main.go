package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
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

func handleRoute(w http.ResponseWriter, r *http.Request) {
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

func main() {

	port := 3000

	http.HandleFunc("/", handleRoute)
	fmt.Printf("Server running (port=%d), route: http://localhost:%d/?loopCount=1&base=8&exp=7.7\n", port, port)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}
