package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	SERVER_PORT = "9988"
	SERVER_TYPE = "tcp"
)

/*
What does cc do:
1. Connect to all host agents
2. Send messages to host agents to update pod state
3. Repeat the following:
	- Get CPU Utilizations from host agents
	- Solve the optimization problem by connection to Gurobi Optimizer
	- Send the CPU shares to the host agents to be applied
*/

type Node struct {
	IP   string
	Pods map[string]string

	connection net.Conn
}

func (n *Node) intializeNode(ip string, pods map[string]string) {
	n.IP = ip
	n.Pods = pods
}

func (n *Node) connect() {
	connection, err := net.Dial(SERVER_TYPE, n.IP+":"+SERVER_PORT)
	if err != nil {
		panic(err)
	}
	n.connection = connection
}

func (n *Node) disconnect() {
	n.connection.Close()
}

func (n *Node) sendMessageAndGetResponse(msg string) string {
	_, err := n.connection.Write([]byte(msg))
	if err != nil {
		slog.Warn("Error sending:" + err.Error())
	}
	slog.Info("Sent: " + msg)
	buffer := make([]byte, 4096)
	mLen, err := n.connection.Read(buffer)
	if err != nil {
		slog.Warn("Error reading:" + err.Error())
	}
	slog.Info("Received: " + string(buffer[:mLen]))
	return string(buffer[:mLen])
}

type CPUUtil struct {
	Node            int
	CPUUtilizations string
}

func main() {

	// Initialize nodes
	nodes := [3]Node{}
	nodes[0].intializeNode("localhost",
		map[string]string{"app1-node1": "pod1uid", "app3-node1": "pod2uid"})
	nodes[1].intializeNode("localhost",
		map[string]string{"app1-node2": "pod3uid", "app2-node2": "pod4uid"})
	nodes[2].intializeNode("localhost",
		map[string]string{"app2-node3": "pod5uid"})

	// Connect to all host agents
	for _, node := range nodes {
		node.connect()
	}

	// Defer disconnecting from all host agents
	defer func() {
		for _, node := range nodes {
			node.disconnect()
		}
	}()

	// Send messages to host agents to update pod state
	for _, node := range nodes {
		msg := "updatePods"
		for pod, uid := range node.Pods {
			msg += " " + pod + ":" + uid
		}
		response := node.sendMessageAndGetResponse(msg)
		if response != "Success" {
			panic("Failed to update pod state on node: " + node.IP)
		}
	}

	// Repeat the following:
	// - Get CPU Utilizations from host agents
	// - Solve the optimization problem by connection to Gurobi Optimizer
	// - Send the CPU shares to the host agents to be applied
	for {

		// - Get CPU Utilizations from host agents
		cpuUtilizationCh := make(chan CPUUtil)
		for i, node := range nodes {
			msg := "getCPUUtilizations"
			go func() {
				cpuUtilizations := node.sendMessageAndGetResponse(msg)
				cpuUtilizationCh <- CPUUtil{i, cpuUtilizations}
			}()
		}
		nodeCPUUtilizations := make([]string, len(nodes))
		for range nodes {
			cpuUtil := <-cpuUtilizationCh
			nodeCPUUtilizations[cpuUtil.Node] = cpuUtil.CPUUtilizations
			slog.Info(fmt.Sprintf("CPU Utilizations [Node %d]: %s",
				cpuUtil.Node, cpuUtil.CPUUtilizations))
		}

		// - Solve the optimization problem by connection to Gurobi Optimizer
		nodeCPUShares := getOptimalCPUShares(nodeCPUUtilizations)

		// - Send the CPU shares to the host agents to be applied
		if nodeCPUShares == nil {
			slog.Warn("Failed to get optimal CPU shares")
		} else {
			for i, node := range nodes {
				msg := "applyCPUShares " + nodeCPUShares[i]
				response := node.sendMessageAndGetResponse(msg)
				if response != "Success" {
					slog.Warn("Failed to apply CPU shares on node: " + node.IP)
				}
			}
		}
	}
}

func getOptimalCPUShares(nodeCPUUtilizations []string) []string {

	// parse cpu utilizations
	appUtils := getPerAppUtilizations(nodeCPUUtilizations)

	// get weights from gurobi
	gurobiResponse := getWeightsFromGurobi(200.0, appUtils)

	// get cpu shares
	nodeCPUShares := getNodeCPUShares(gurobiResponse)

	return nodeCPUShares
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func getPerAppUtilizations(nodeCPUUtilizations []string) map[int]float64 {

	appUtils := make(map[int]float64)
	for _, cpuUtil := range nodeCPUUtilizations {

		// example cpuUtil to parse: "cpuUtilizations app1-node1:45 app2-node1:69"

		cpuUtilStrs := strings.Split(cpuUtil, " ")[1:]
		for _, cpuUtilStr := range cpuUtilStrs {

			util := strings.Split(cpuUtilStr, ":")

			appNumStr := (util[0][3:4])
			appNum, err := strconv.Atoi(appNumStr)
			check(err)

			podUtil, err := strconv.ParseFloat(util[1], 64)
			check(err)

			appUtils[appNum] += podUtil
		}

	}
	return appUtils
}

type GurobiResponse struct {
	Status    int     `json:"status"`
	App1Node1 float64 `json:"t00"`
	App1Node2 float64 `json:"t01"`
	App2Node2 float64 `json:"t11"`
	App2Node3 float64 `json:"t12"`
	App3Node1 float64 `json:"t20"`
}

func getWeightsFromGurobi(
	hostCap float64, appUtils map[int]float64) string {

	baseURL := "http://localhost:5000"
	resource := "/"
	params := url.Values{}
	params.Add("host_cap", fmt.Sprintf("%f", hostCap))
	params.Add("t0", fmt.Sprintf("%d", appUtils[1]))
	params.Add("t1", fmt.Sprintf("%d", appUtils[2]))
	params.Add("t2", fmt.Sprintf("%d", appUtils[3]))

	u, _ := url.ParseRequestURI(baseURL)
	u.Path = resource
	u.RawQuery = params.Encode()
	urlStr := fmt.Sprintf("%v", u)

	res, err := http.Get(urlStr)
	check(err)

	resBody, err := io.ReadAll(res.Body)
	check(err)

	return string(resBody)
}

func getNodeCPUShares(gurobiResponse string) []string {

	var response GurobiResponse
	err := json.Unmarshal([]byte(gurobiResponse), &response)
	check(err)

	if response.Status != 2 {
		slog.Warn("gurobi returned status %d", response.Status)
		return nil
	} else {
		nodeCPUShares := make([]string, 3)
		nodeCPUShares[0] = fmt.Sprintf("%s:%f %s:%f",
			"app1-node1", response.App1Node1, "app3-node1", response.App3Node1)
		nodeCPUShares[1] = fmt.Sprintf("%s:%f %s:%f",
			"app1-node2", response.App1Node2, "app2-node2", response.App2Node2)
		nodeCPUShares[2] = fmt.Sprintf("%s:%f",
			"app2-node3", response.App2Node3)

		return nodeCPUShares
	}
}
