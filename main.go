package main

import (
	"CanglingAgent/agent"
	pb "CanglingAgent/agent"
	"CanglingAgent/config"
	"fmt"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var port int32 = 50051

type CanglingServer struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Author        string `json:"author"`
	Email         string `json:"email"`
	CompileTime   string `json:"compileTime"`
	GitHash       string `json:"gitHash"`
	LatestVersion string `json:"latestVersion"`
}

var AppVersion = "1.0.8"              // Current application version
var BuildTime = "2006-01-02 15:04:05" // Build timestamp in milliseconds (UTC) injected by build script
var GitHash = "unknown"               // Git hash injected by build script
var canglingServer = CanglingServer{  // Use api.CanglingServer from the api package
	Name:        "Cangling Agent",
	Version:     AppVersion,
	Author:      "Zhang JianShe",
	Email:       "zhangjianshe@gmail.com",
	CompileTime: BuildTime,
	GitHash:     GitHash,
}

var registerUrl = ""
var registerToken = ""

func init() {
	printBanner()
	err := Config.Read("")
	if err != nil {
		log.Fatalf("Error: %v\n", err)
	}
	serverCmd.Flags().Int32VarP(&port, "port", "p", 0, "Port to listen on")

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(registerCmd)

	registerCmd.Flags().StringVarP(&registerUrl, "server", "", "", "api server's url")
	registerCmd.Flags().StringVarP(&registerToken, "token", "", "", "api register token")
}

var Config config.Config

func printBanner() {

}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "CanglingAgent",
	Short: "agent for CanglingServer",
	Long:  `cangling agent is runing on a worker node, communicate to the cangling api server.`,
	Run: func(cmd *cobra.Command, args []string) {
		serverCmd.Run(cmd, args)
	},
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the Cangling Server",
	Long:  `Starts the Cangling Server`,
	Run:   startAgent, // The function that actually starts the server
}

// versionCmd represents the 'version' subcommand
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of CanlingAgent",
	Long:  `All software has versions. This is CanlingServer's.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", canglingServer.Name, canglingServer.Version)
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register Agent to the CanglingServer",
	Run: func(cmd *cobra.Command, args []string) {

		nodeId, err := agent.Register(registerUrl, registerToken, Config.Server.Port, canglingServer.Version)
		if err != nil {
			log.Printf("Error %v", err)
		} else {
			Config.Server.AgentId = nodeId
			Config.Server.ServerUrl = registerUrl
			err := Config.Write("")
			if err != nil {
				log.Fatalf("Error: %v", err)
			} else {
				log.Printf("register success %v\n", nodeId)
			}
		}
	},
}

func startAgent(cmd *cobra.Command, args []string) {
	if port == 0 {
		port = Config.Server.Port
	} else {
		Config.Server.Port = port
		// Save port change immediately
		if err := Config.Write(""); err != nil {
			log.Fatalf("Failed to write config after port change: %v", err)
		}
	}

	// 1. Create a TCP listener
	lis, err := net.Listen("tcp", ":"+fmt.Sprintf("%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// 2. Create the gRPC server instance
	s := grpc.NewServer()
	pb.RegisterAgentServiceServer(s, pb.NewGrpcServer())
	reflection.Register(s)

	// 3. Start gRPC Server (Non-blocking)
	go func() {
		fmt.Printf("gRPC server listening on :%d\n", port)
		if err := s.Serve(lis); err != nil {
			// Use Errorf instead of Fatalf here, since we are in a separate goroutine
			log.Printf("gRPC failed to serve: %v", err)
		}
	}()

	// 4. Setup Periodic Agent Reporting
	// Create a channel to signal when to stop the reporting goroutine
	done := make(chan struct{})
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop() // Ensure ticker is stopped when startAgent exits

	// Run the scheduler loop in a non-blocking goroutine
	go func() {
		log.Println("Starting periodic agent report (every 5s)...")
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// FIX: The return statement was removed here. The loop continues.
				err2 := agent.ReportAgentToServer(Config, canglingServer.Version)
				if err2 != nil {
					log.Printf("Error during agent report: %v", err2)
				}
			}
		}
	}()

	// 5. Wait for Graceful Shutdown Signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit // Block until signal is received

	// 6. Gracefully Shut Down
	log.Println("Received shutdown signal. Stopping agent report...")
	close(done) // Signal the reporting goroutine to stop

	log.Println("Shutting down gRPC server...")
	s.GracefulStop()
	log.Println("Server exited successfully.")
}

func main() {
	// Execute the root command. Cobra will handle parsing args and calling the right command.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
