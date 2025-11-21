package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Version string `json:"version"`
	Port    int32  `json:"port"`
}

type NodeInfo struct {
	Version   string `json:"version"`
	Memory    int64  `json:"memory"`
	TaskCount int32  `json:"taskCount"`
	Ip        string `json:"ip"`
	Name      string `json:"name"`
	StartAt   string `json:"startAt"`
}

type JobInfo struct {
	Name          string   `json:"name"`
	StartAt       string   `json:"startAt"`
	Image         string   `json:"image"`
	Id            string   `json:"id"`
	GpuRequest    []int    `json:"gpus"`
	MemoryRequest int      `json:"memory"` // Assuming MB
	VolumeRequest []string `json:"volumes"`
	Envs          []string `json:"envs"`
}

// WriteOk marshals data into a successful JSON response
func WriteOk(writer http.ResponseWriter, data interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	result, err := json.Marshal(Ok(data))
	if err != nil {
		log.Printf("Error marshalling success response: %v", err)
		errorJson, _ := json.Marshal(Error(http.StatusInternalServerError, "Internal server error during response marshalling"))
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write(errorJson)
		return
	}
	_, _ = writer.Write(result)
}

// WriteError marshals an error into a JSON response
func WriteError(writer http.ResponseWriter, code int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK) // We usually return 200 OK with a non-zero Code in body, or actual status code. Keeping your pattern.
	result, err := json.Marshal(Error(code, message))
	if err != nil {
		log.Printf("Error marshalling error response: %v", err)
		fallbackErrorJson, _ := json.Marshal(Error(http.StatusInternalServerError, "Internal server error"))
		_, _ = writer.Write(fallbackErrorJson)
		return
	}
	_, _ = writer.Write(result)
}

// WriteImage writes an image buffer as a PNG response
func WriteImage(writer http.ResponseWriter, buffer bytes.Buffer, cacheTime time.Duration) {
	writer.Header().Set("Content-Type", "image/png")
	writer.WriteHeader(http.StatusOK)
	writer.Header().Set("Content-Length", strconv.Itoa(len(buffer.Bytes())))
	if cacheTime > 0 {
		writer.Header().Set("Cache-Control", "public,max-age="+cacheTime.String())
	}
	_, _ = writer.Write(buffer.Bytes())
}
func WriteBytes(writer http.ResponseWriter, contentType string, buffer []byte, cacheTime time.Duration) {
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(http.StatusOK)
	if cacheTime > 0 {
		writer.Header().Set("Cache-Control", "public,max-age="+cacheTime.String())
	}
	writer.Header().Set("Content-Length", strconv.Itoa(len(buffer)))
	_, _ = writer.Write(buffer)
}

// WriteHtml writes HTML content
func WriteHtml(writer http.ResponseWriter, content []byte) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
	_, _ = writer.Write(content)
}

func NewServer(port int32) *Server {
	return &Server{
		Version: "1.0.0",
		Port:    port,
	}
}

func (s *Server) Initialize(r *mux.Router) {
	r.HandleFunc("/api/v1/node/info", s.version).Methods("GET")
	r.HandleFunc("/api/v1/task/ls", s.ls).Methods("GET")
	r.HandleFunc("/api/v1/task/start", s.start).Methods("POST")
	r.HandleFunc("/api/v1/task/log", s.log).Methods("GET")
	r.HandleFunc("/api/v1/task/stop", s.stop).Methods("GET")
}

// version echo the version of this server
func (s *Server) version(w http.ResponseWriter, r *http.Request) {
	WriteOk(w, s.Version)
}

// ls lists all containers
func (s *Server) ls(w http.ResponseWriter, r *http.Request) {
	command := exec.Command("docker", "ps", "-a")
	command.Stdout = w
	err := command.Run()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

// start starts a container job based on JSON body
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	// 1. Parse the JSON body into JobInfo struct
	var job JobInfo
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	// 2. Basic Validation
	if job.Image == "" {
		WriteError(w, http.StatusBadRequest, "Field 'image' is required")
		return
	}

	// 3. Construct Docker arguments dynamically
	// Start with base args: "run", "--rm", "-d" (detached)
	args := []string{"run", "--rm", "-d"}

	// Container Name
	if job.Name != "" {
		args = append(args, "--name", job.Name)
	}

	// Environment Variables
	for _, env := range job.Envs {
		args = append(args, "-e", env)
	}

	// Volumes
	for _, vol := range job.VolumeRequest {
		args = append(args, "-v", vol)
	}

	// Memory Limit (assuming value is in MB, appending 'm')
	if job.MemoryRequest > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", job.MemoryRequest))
	}

	// GPU Support
	if len(job.GpuRequest) > 0 {
		// Construct GPU string, e.g., "device=0,1"
		var gpuIDs []string
		for _, id := range job.GpuRequest {
			gpuIDs = append(gpuIDs, strconv.Itoa(id))
		}
		args = append(args, "--gpus", fmt.Sprintf("device=%s", strings.Join(gpuIDs, ",")))
	}

	// Labels (Metadata)
	args = append(args, "--label", "managed-by=cangling-api")
	if job.Id != "" {
		args = append(args, "--label", fmt.Sprintf("job-id=%s", job.Id))
	}

	// Image must be the last argument for `docker run`
	args = append(args, job.Image)

	// 4. Execute
	command := exec.Command("docker", args...)

	// Capture stdout/stderr of the 'docker run' command itself
	var commandOutput bytes.Buffer
	var commandError bytes.Buffer
	command.Stdout = &commandOutput
	command.Stderr = &commandError

	err := command.Run()
	if err != nil {
		errorMessage := fmt.Sprintf("Docker run failed: %s", err.Error())
		if commandError.Len() > 0 {
			errorMessage += fmt.Sprintf(" | Docker STDERR: %s", commandError.String())
		}
		WriteError(w, http.StatusInternalServerError, errorMessage)
		return
	}

	// Return success message with the container ID
	WriteOk(w, fmt.Sprintf("Job started successfully. ID: %s", strings.TrimSpace(commandOutput.String())))
}

// stop explicitly stops and removes a running container.
// Accepts '?name=' query param, defaults to "agent-test"
func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	targetName := r.URL.Query().Get("name")
	if targetName == "" {
		targetName = "agent-test"
	}

	command := exec.Command("docker", "stop", targetName)

	var commandOutput bytes.Buffer
	var commandError bytes.Buffer
	command.Stdout = &commandOutput
	command.Stderr = &commandError

	err := command.Run()
	if err != nil {
		errorMessage := fmt.Sprintf("Docker stop failed: %s", err.Error())
		if commandError.Len() > 0 {
			if bytes.Contains(commandError.Bytes(), []byte("No such container")) {
				WriteOk(w, fmt.Sprintf("Container '%s' was already stopped or does not exist.", targetName))
				return
			}
			errorMessage += fmt.Sprintf(" | Docker STDERR: %s", commandError.String())
		}
		WriteError(w, http.StatusInternalServerError, errorMessage)
		return
	}

	WriteOk(w, fmt.Sprintf("Container '%s' stopped successfully: %s", targetName, commandOutput.String()))
}

// log streams logs. Accepts '?name=' query param, defaults to "agent-test"
func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	targetName := r.URL.Query().Get("name")
	if targetName == "" {
		targetName = "agent-test"
	}

	// Check for Flusher first
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "Streaming unsupported by ResponseWriter")
		return
	}

	command := exec.CommandContext(r.Context(), "docker", "logs", "-f", targetName)

	// 1. Direct pipe STDOUT to the ResponseWriter (w).
	command.Stdout = w

	// 2. Separate buffer for Docker's STDERR (errors)
	var dockerStderr bytes.Buffer
	command.Stderr = &dockerStderr

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Send an immediate preamble and flush the headers.
	fmt.Fprintf(w, "--- Log Stream Initialized for '%s' (Forcing flush every 100ms) ---\n", targetName)
	flusher.Flush()

	// Manual Flusher Goroutine
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			if r.Context().Err() != nil {
				return
			}
			flusher.Flush()
		}
	}()

	// Start the command non-blocking.
	if err := command.Start(); err != nil {
		log.Printf("ERROR: Failed to start docker logs process: %v", err)
		fmt.Fprintf(w, "\n--- CRITICAL STARTUP ERROR ---\nFailed to start docker logs: %v\n", err)
		flusher.Flush()
		return
	}

	// Wait for the command to finish.
	err := command.Wait()

	log.Print("INFO: Log command finished or client disconnected.")

	if err != nil && !errors.Is(r.Context().Err(), context.Canceled) {
		errorMessage := fmt.Sprintf("Docker logs command failed: %v", err)
		if dockerStderr.Len() > 0 {
			errorMessage += fmt.Sprintf("\nDocker STDERR:\n%s", dockerStderr.String())
		}
		fmt.Fprintf(w, "\n--- LOG STREAM ERROR ---\n%s\n", errorMessage)
	}

	// Final cleanup flush
	flusher.Flush()
}

// Result struct defines a standard API response format
type Result struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Ok creates a successful API result
func Ok(data interface{}) Result {
	return Result{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// Error creates an error API result
func Error(code int, message string) Result {
	return Result{
		Code:    code,
		Message: message,
		Data:    nil,
	}
}
