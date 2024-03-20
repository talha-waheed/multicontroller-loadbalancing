package main

import (
	"fmt"
	"net"
)

const (
	SERVER_HOST = "localhost"
	SERVER_PORT = "9988"
	SERVER_TYPE = "tcp"
)

func main() {
	//establish connection
	connection, err := net.Dial(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		panic(err)
	}

	msgs := []string{
		"updatePods pod1:pod1uid pod2:pod2uid",
		"applyCPUShares pod1:45 pod2:67",
		"applyCPUShares pod1:33 pod2:55",
		"getCPUUtilizations",
	}

	for _, msg := range msgs {

		_, err = connection.Write(
			[]byte(msg))
		if err != nil {
			fmt.Println("Error sending:", err.Error())
		}
		buffer := make([]byte, 1024)
		mLen, err := connection.Read(buffer)
		if err != nil {
			fmt.Println("Error reading:", err.Error())
		}
		fmt.Println("Received: ", string(buffer[:mLen]))
	}

	defer connection.Close()
}
