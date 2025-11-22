package agent

import (
	"CanglingAgent/config"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pbnjay/memory"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

type ApiResult struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type Gpu struct {
	Module    string `json:"module"`
	Memory    int64  `json:"memory"`
	ProcessId string `json:"processId"`
	TaskId    string `json:"taskId"`
	Slot      int32  `json:"slot"`
}

type WorkNode struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	InternalIp   string `json:"internalIp"`
	Port         int32  `json:"port"`
	Os           string `json:"os"`
	Architecture string `json:"architecture"`
	AgentVersion string `json:"agentVersion"`
	Memory       uint64 `json:"memory"`
	Storage      uint64 `json:"storage"`
	Pods         uint   `json:"pods"`
	MemoryFree   uint64 `json:"memoryFree"`
	StorageFree  uint64 `json:"storageFree"`
	RunningPods  uint   `json:"runningPods"`
	Online       bool   `json:"online"`
	CreateTime   int64  `json:"createTime"`
	OnlineTime   int64  `json:"onlineTime"`
	Gpus         []Gpu  `json:"gpus"`
}
type RegisterRequest struct {
	RegisterKey string   `json:"registerKey"`
	Node        WorkNode `json:"node"`
}

func ReportAgentToServer(config config.Config, version string) error {
	if config.Server.ServerUrl == "" {
		return fmt.Errorf("this agent dose not have register to a server")
	}
	var hostName, err = os.Hostname()
	if err != nil {
		hostName = ""
	}

	var request = RegisterRequest{
		Node: WorkNode{
			Id:           config.Server.AgentId,
			Name:         hostName,
			Memory:       getMemory(),
			MemoryFree:   getMemoryFree(),
			Online:       true,
			Architecture: runtime.GOARCH,
			Os:           runtime.GOOS,
			AgentVersion: version,
		},
	}
	result := &ApiResult{}
	err = postJSON(config.Server.ServerUrl, request, result)
	if err != nil {
		return err
	}
	if result.Code != 200 {
		return fmt.Errorf("%s", result.Message)
	}
	return nil
}

// Register an agent to the cangling server
func Register(url string, token string, port int32, version string) (string, error) {
	if url != "" && token != "" {
		hostName, err := os.Hostname()
		if err != nil {
			return "", err
		}
		ip, err := getLocalIP()
		if err != nil {
			return "", err
		}
		var request = RegisterRequest{
			RegisterKey: token,
			Node: WorkNode{
				Id:           "",
				Name:         hostName,
				InternalIp:   ip,
				Port:         port,
				Memory:       getMemory(),
				MemoryFree:   getMemoryFree(),
				Architecture: runtime.GOARCH,
				Os:           runtime.GOOS,
				AgentVersion: version,
			},
		}
		result := &ApiResult{}
		err = postJSON(url, request, result)
		if err != nil {
			return "", err
		}
		if result.Code != 200 {
			return "", fmt.Errorf("%s", result.Message)
		} else {
			dataMap, ok := result.Data.(map[string]interface{})
			if !ok {
				fmt.Println("Error: Data field is not a map[string]interface{}")
				return "", fmt.Errorf("%s", "没有返回数据")
			}
			nodeMap, ok := dataMap["node"].(map[string]interface{}) // JSON numbers default to float64
			if !ok {
				fmt.Println("return node is not a map[string]interface{}")
				return "", fmt.Errorf("%s", "node data is not a map[string]interface{}")
			}
			nodeId, ok := nodeMap["id"].(string)
			if !ok {
				fmt.Println("node id is not a string")
				return "", fmt.Errorf("%s", "node id is not a string")
			}
			return nodeId, nil
		}

	} else {
		return "", errors.New("url or token required")
	}
	return "", nil
}

func getMemory() uint64 {
	return uint64(float64(memory.TotalMemory()) / 1024 / 1024 / 1024)
}

func getMemoryFree() uint64 {
	return uint64(float64(memory.FreeMemory()) / 1024 / 1024 / 1024)
}

func postJSON(url string, payload interface{}, result interface{}) error {
	// Convert the payload struct to a JSON byte slice
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling request payload: %w", err)
	}

	// 1. Send the POST Request
	// bytes.NewBuffer(requestBody) converts the byte slice into an io.Reader
	// The http.Post function automatically sets the Content-Type header to "application/x-www-form-urlencoded"
	// To explicitly set the Content-Type to "application/json", we should use http.NewRequest and http.Client.Do().

	// --- Using http.Client for better control (recommended) ---

	client := &http.Client{
		Timeout: 10 * time.Second, // Set a timeout for the request
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Explicitly set the Content-Type header to indicate we are sending JSON
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	// Ensure the response body is closed to prevent resource leaks
	defer resp.Body.Close()

	// 2. Check the HTTP status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// Read the body into a byte slice for a detailed error message
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("received non-successful status code: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// 3. Decode the JSON Response Body
	// We read directly from resp.Body into the result interface (a pointer)
	err = json.NewDecoder(resp.Body).Decode(result)
	if err != nil {
		return fmt.Errorf("error decoding response body: %w", err)
	}

	return nil
}
func getLocalIP() (string, error) {
	// 1. Get a list of all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	// 2. Iterate through interfaces to find a valid IP
	for _, iface := range ifaces {
		// Skip loopback (lo) and interfaces that are down
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Get all addresses for the interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue // Skip if we can't get addresses
		}

		// Iterate through addresses
		for _, addr := range addrs {
			// Check if it's an IPv4 address
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Ensure the IP is not a loopback address and is IPv4
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // Must be IPv4
			}

			// We found a non-loopback IPv4 address
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("no suitable IP address found")
}
