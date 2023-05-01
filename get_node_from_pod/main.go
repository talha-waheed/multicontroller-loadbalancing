package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	// get env variable named "HOSTNAME"

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	// repeatedly print hostname
	for {
		fmt.Println(hostname)
		fmt.Printf("MY_NODE_NAME: %s", os.Getenv("MY_NODE_NAME"))
		fmt.Printf("MY_POD_NAME: %s", os.Getenv("MY_POD_NAME"))
		fmt.Printf("MY_POD_NAMESPACE: %s", os.Getenv("MY_POD_NAMESPACE"))
		fmt.Printf("MY_POD_IP: %s", os.Getenv("MY_POD_IP"))
		fmt.Printf("MY_POD_SERVICE_ACCOUNT: %s", os.Getenv("MY_POD_SERVICE_ACCOUNT"))
		time.Sleep(10 * time.Second)
	}
}
