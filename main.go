package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "proxy-v6",
		Short: "Distributed IPv6 proxy system",
		Long: `A distributed proxy system that manages IPv6 addresses across multiple nodes.
		
Available commands:
  agent       - Run as an agent on a node to manage local IPv6 proxies
  coordinator - Run as a coordinator to manage multiple agents
  monitor     - Launch TUI for monitoring the system`,
	}
	
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Run proxy agent",
		Run: func(cmd *cobra.Command, args []string) {
			runCommand("agent", args)
		},
	}
	
	coordinatorCmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Run coordinator service",
		Run: func(cmd *cobra.Command, args []string) {
			runCommand("coordinator", args)
		},
	}
	
	monitorCmd := &cobra.Command{
		Use:   "monitor",
		Short: "Run TUI monitor",
		Run: func(cmd *cobra.Command, args []string) {
			runCommand("monitor", args)
		},
	}
	
	agentCmd.Flags().StringP("config", "c", "", "config file path")
	agentCmd.Flags().IntP("port", "p", 8080, "API listen port")
	agentCmd.Flags().IntP("proxy-start", "", 10000, "Starting port for proxy instances")
	agentCmd.Flags().IntP("proxy-end", "", 20000, "Ending port for proxy instances")
	agentCmd.Flags().StringP("coordinator", "", "", "Coordinator URL")
	agentCmd.Flags().IntP("metrics-port", "m", 9090, "Metrics port")
	
	coordinatorCmd.Flags().StringP("config", "c", "", "config file path")
	coordinatorCmd.Flags().IntP("port", "p", 8081, "API listen port")
	coordinatorCmd.Flags().IntP("proxy-port", "", 8888, "Proxy listen port")
	coordinatorCmd.Flags().IntP("metrics-port", "m", 9091, "Metrics port")
	
	monitorCmd.Flags().StringP("coordinator", "c", "http://localhost:8081", "Coordinator URL")
	
	rootCmd.AddCommand(agentCmd, coordinatorCmd, monitorCmd)
	
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runCommand(command string, args []string) {
	fmt.Printf("Please build and run the specific command binary:\n")
	fmt.Printf("  go run cmd/%s/main.go %s\n", command, joinArgs(args))
	fmt.Printf("Or build first:\n")
	fmt.Printf("  go build -o bin/%s cmd/%s/main.go\n", command, command)
	fmt.Printf("  ./bin/%s %s\n", command, joinArgs(args))
}

func joinArgs(args []string) string {
	result := ""
	for _, arg := range args {
		if result != "" {
			result += " "
		}
		result += arg
	}
	return result
}