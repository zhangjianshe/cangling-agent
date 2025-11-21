package agent

import (
	"bytes"
	"context"
	"fmt"
	_ "io"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GrpcServer implements the protobuf AgentServiceServer interface
type GrpcServer struct {
	UnimplementedAgentServiceServer
	Version string
}

func NewGrpcServer() *GrpcServer {
	return &GrpcServer{
		Version: "1.0.0",
	}
}

// GetVersion implements GET /api/v1/node/info
func (s *GrpcServer) GetVersion(ctx context.Context, req *Empty) (*VersionResponse, error) {
	return &VersionResponse{Version: s.Version}, nil
}

// ListTasks implements GET /api/v1/task/ls
func (s *GrpcServer) ListTasks(ctx context.Context, req *Empty) (*ListTasksResponse, error) {
	command := exec.Command("docker", "ps", "-a")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to list tasks: %v", err)
	}
	return &ListTasksResponse{Output: string(output)}, nil
}

// StartTask implements POST /api/v1/task/start
func (s *GrpcServer) StartTask(ctx context.Context, req *StartTaskRequest) (*StartTaskResponse, error) {
	// 1. Validation
	if req.Image == "" {
		return nil, status.Error(codes.InvalidArgument, "Field 'image' is required")
	}

	// 2. Construct Docker arguments
	args := []string{"run", "--rm", "-d"}

	if req.Name != "" {
		args = append(args, "--name", req.Name)
	}

	for _, env := range req.Envs {
		args = append(args, "-e", env)
	}

	for _, vol := range req.Volumes {
		args = append(args, "-v", vol)
	}

	if req.MemoryMb > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", req.MemoryMb))
	}

	if len(req.Gpus) > 0 {
		var gpuIDs []string
		for _, id := range req.Gpus {
			gpuIDs = append(gpuIDs, strconv.Itoa(int(id)))
		}
		args = append(args, "--gpus", fmt.Sprintf("device=%s", strings.Join(gpuIDs, ",")))
	}

	// Labels
	args = append(args, "--label", "managed-by=cangling-grpc")
	if req.Id != "" {
		args = append(args, "--label", fmt.Sprintf("job-id=%s", req.Id))
	}

	args = append(args, req.Image)

	// 3. Execute
	command := exec.CommandContext(ctx, "docker", args...)

	var commandOutput bytes.Buffer
	var commandError bytes.Buffer
	command.Stdout = &commandOutput
	command.Stderr = &commandError

	if err := command.Run(); err != nil {
		errMsg := fmt.Sprintf("Docker run failed: %s", err.Error())
		if commandError.Len() > 0 {
			errMsg += fmt.Sprintf(" | Docker STDERR: %s", commandError.String())
		}
		return nil, status.Error(codes.Internal, errMsg)
	}

	containerID := strings.TrimSpace(commandOutput.String())
	return &StartTaskResponse{
		ContainerId: containerID,
		Message:     fmt.Sprintf("Job started successfully. ID: %s", containerID),
	}, nil
}

// StopTask implements GET /api/v1/task/stop
func (s *GrpcServer) StopTask(ctx context.Context, req *StopTaskRequest) (*StopTaskResponse, error) {
	targetName := req.Name
	if targetName == "" {
		targetName = "agent-test"
	}

	command := exec.CommandContext(ctx, "docker", "stop", targetName)
	var commandError bytes.Buffer
	command.Stderr = &commandError

	if err := command.Run(); err != nil {
		// Handle "No such container" gracefully
		if bytes.Contains(commandError.Bytes(), []byte("No such container")) {
			return &StopTaskResponse{
				Message: fmt.Sprintf("Container '%s' was already stopped or does not exist.", targetName),
			}, nil
		}

		errMsg := fmt.Sprintf("Docker stop failed: %s", err.Error())
		if commandError.Len() > 0 {
			errMsg += fmt.Sprintf(" | Docker STDERR: %s", commandError.String())
		}
		return nil, status.Error(codes.Internal, errMsg)
	}

	return &StopTaskResponse{
		Message: fmt.Sprintf("Container '%s' stopped successfully", targetName),
	}, nil
}

// StreamLogs implements GET /api/v1/task/log
func (s *GrpcServer) StreamLogs(req *StreamLogsRequest, stream AgentService_StreamLogsServer) error {
	targetName := req.Name
	if targetName == "" {
		targetName = "agent-test"
	}

	// 1. Send initialization message
	initMsg := fmt.Sprintf("--- Log Stream Initialized for '%s' ---\n", targetName)
	if err := stream.Send(&LogChunk{Data: []byte(initMsg)}); err != nil {
		return err
	}

	// 2. Prepare command linked to stream context
	// When the client disconnects, stream.Context() is canceled, killing the command.
	cmd := exec.CommandContext(stream.Context(), "docker", "logs", "-f", targetName)

	// 3. Pipe Stdout to the gRPC stream
	// We use a custom writer to bridge io.Writer -> gRPC Stream
	logWriter := &LogStreamWriter{Stream: stream}
	cmd.Stdout = logWriter

	// Capture stderr separately for final error reporting
	var dockerStderr bytes.Buffer
	cmd.Stderr = &dockerStderr

	// 4. Run the command
	// Unlike the HTTP handler, we don't need a manual ticker/flusher here.
	// gRPC streams flush messages individually.
	if err := cmd.Run(); err != nil {
		// Check if error is due to client disconnect
		if stream.Context().Err() != nil {
			log.Println("Client disconnected from log stream")
			return nil // Expected behavior
		}

		// Send error details to client before closing
		errMsg := fmt.Sprintf("\n--- LOG STREAM ERROR ---\nDocker command failed: %v", err)
		if dockerStderr.Len() > 0 {
			errMsg += fmt.Sprintf("\nDocker STDERR:\n%s", dockerStderr.String())
		}
		// Try to send the error message to the user
		_ = stream.Send(&LogChunk{Data: []byte(errMsg)})

		return status.Errorf(codes.Internal, "Log stream failed: %v", err)
	}

	return nil
}

// LogStreamWriter is a helper to adapt io.Writer to gRPC stream.Send
type LogStreamWriter struct {
	Stream AgentService_StreamLogsServer
}

func (w *LogStreamWriter) Write(p []byte) (n int, err error) {
	// gRPC Send is likely not thread-safe, but exec.Cmd writes sequentially to stdout.
	// Note: We copy the bytes because 'p' might be reused by the caller.
	data := make([]byte, len(p))
	copy(data, p)

	if err := w.Stream.Send(&LogChunk{Data: data}); err != nil {
		return 0, err
	}
	return len(p), nil
}
