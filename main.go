package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "CanglingAgent/agent"
)

const (
	port = ":50051" // Standard gRPC port
)

func main() {
	// 1. Create a TCP listener
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 2. Create the gRPC server instance
	s := grpc.NewServer()

	// 3. Register your server implementation
	// NewGrpcServer() is defined in grpc_server.go
	pb.RegisterAgentServiceServer(s, pb.NewGrpcServer())

	// 4. Enable Server Reflection (optional, but highly recommended for debugging with grpcurl)
	reflection.Register(s)

	// 5. Handle Graceful Shutdown
	go func() {
		fmt.Printf("gRPC server listening on %s\n", port)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal (Ctrl+C)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gRPC server...")
	s.GracefulStop()
	log.Println("Server exited")
}
