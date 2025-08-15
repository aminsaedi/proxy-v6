package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"proxy-v6/internal/ipscanner"
	"proxy-v6/internal/proxy"
	"proxy-v6/pkg/models"
	"proxy-v6/pkg/version"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logger *logrus.Logger
	cfg    models.AgentConfig
)

func main() {
	logger = logrus.New()
	
	// Use text formatter for better readability during debugging
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	
	// Set debug level by default for better visibility
	logger.SetLevel(logrus.DebugLevel)
	
	rootCmd := &cobra.Command{
		Use:   "agent",
		Short: "IPv6 proxy agent for managing tinyproxy instances",
		Run:   runAgent,
	}
	
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.GetVersion())
		},
	}
	
	rootCmd.AddCommand(versionCmd)
	
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
	rootCmd.PersistentFlags().IntP("port", "p", 8080, "API listen port")
	rootCmd.PersistentFlags().IntP("proxy-start", "", 10000, "Starting port for proxy instances")
	rootCmd.PersistentFlags().IntP("proxy-end", "", 20000, "Ending port for proxy instances")
	rootCmd.PersistentFlags().StringP("coordinator", "", "", "Coordinator URL")
	rootCmd.PersistentFlags().IntP("metrics-port", "m", 9090, "Metrics port")
	rootCmd.PersistentFlags().StringSlice("allowed-ips", []string{}, "IPs allowed to connect to proxies (comma-separated)")
	rootCmd.PersistentFlags().StringP("proxy-mode", "", "restricted", "Proxy access mode: 'open' (allow all) or 'restricted' (allow only specified IPs)")
	rootCmd.PersistentFlags().StringP("log-level", "l", "debug", "Log level: debug, info, warn, error")
	
	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		logger.Fatalf("Failed to bind flags: %v", err)
	}
	
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal(err)
	}
}

func runAgent(cmd *cobra.Command, args []string) {
	// Set log level
	logLevel := viper.GetString("log-level")
	switch logLevel {
	case "debug":
		logger.SetLevel(logrus.DebugLevel)
	case "info":
		logger.SetLevel(logrus.InfoLevel)
	case "warn":
		logger.SetLevel(logrus.WarnLevel)
	case "error":
		logger.SetLevel(logrus.ErrorLevel)
	default:
		logger.SetLevel(logrus.DebugLevel)
	}
	
	configFile := viper.GetString("config")
	if configFile != "" {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			logger.Warnf("Failed to read config file: %v", err)
		}
	}
	
	cfg = models.AgentConfig{
		ListenPort:     viper.GetInt("port"),
		ProxyStartPort: viper.GetInt("proxy-start"),
		ProxyEndPort:   viper.GetInt("proxy-end"),
		CoordinatorURL: viper.GetString("coordinator"),
		MetricsPort:    viper.GetInt("metrics-port"),
		ExcludeInterfaces: []string{"docker", "veth", "br-"},
		AllowedIPs:     viper.GetStringSlice("allowed-ips"),
		ProxyMode:      viper.GetString("proxy-mode"),
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	scanner := ipscanner.NewScanner(logger, cfg.ExcludeInterfaces)
	manager := proxy.NewManager(logger, cfg.ProxyStartPort, cfg.ProxyEndPort)
	
	// Configure access control
	if cfg.ProxyMode == "restricted" {
		// Auto-detect coordinator IP if not explicitly set
		allowedIPs := cfg.AllowedIPs
		if cfg.CoordinatorURL != "" && len(allowedIPs) == 0 {
			// Extract coordinator IP from URL
			if u, err := url.Parse(cfg.CoordinatorURL); err == nil {
				if host, _, err := net.SplitHostPort(u.Host); err == nil {
					allowedIPs = append(allowedIPs, host)
				} else {
					// No port in URL
					allowedIPs = append(allowedIPs, u.Hostname())
				}
			}
		}
		manager.SetAccessControl(allowedIPs, cfg.ProxyMode)
		logger.Infof("Proxy access mode: %s, Allowed IPs: %v", cfg.ProxyMode, allowedIPs)
	} else {
		manager.SetAccessControl(cfg.AllowedIPs, cfg.ProxyMode)
		logger.Warn("Proxy access mode: open - proxies will accept connections from anywhere!")
	}
	
	logger.Info("Scanning for IPv6 addresses...")
	ipv6Addresses, err := scanner.ScanIPv6Addresses()
	if err != nil {
		logger.Fatalf("Failed to scan IPv6 addresses: %v", err)
	}
	
	logger.Infof("Found %d public IPv6 addresses", len(ipv6Addresses))
	
	for _, ipv6 := range ipv6Addresses {
		instance, err := manager.StartProxy(ctx, ipv6)
		if err != nil {
			logger.Errorf("Failed to start proxy for %s: %v", ipv6.IP.String(), err)
			continue
		}
		logger.Infof("Started proxy: %s", instance.ID)
	}
	
	router := setupAPIRouter(manager)
	
	go func() {
		metricsRouter := gin.New()
		metricsRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))
		logger.Infof("Starting metrics server on port %d", cfg.MetricsPort)
		if err := metricsRouter.Run(fmt.Sprintf(":%d", cfg.MetricsPort)); err != nil {
			logger.Errorf("Metrics server error: %v", err)
		}
	}()
	
	if cfg.CoordinatorURL != "" {
		go reportToCoordinator(manager)
	}
	
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ListenPort),
		Handler: router,
	}
	
	go func() {
		logger.Infof("Starting API server on port %d", cfg.ListenPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("API server error: %v", err)
		}
	}()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	
	logger.Info("Shutting down...")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("Server shutdown error: %v", err)
	}
	
	for _, instance := range manager.GetInstances() {
		if err := manager.StopProxy(instance.ID); err != nil {
			logger.Errorf("Failed to stop proxy %s: %v", instance.ID, err)
		}
	}
}

func setupAPIRouter(manager *proxy.Manager) *gin.Engine {
	router := gin.Default()
	
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})
	
	router.GET("/proxies", func(c *gin.Context) {
		instances := manager.GetInstances()
		c.JSON(200, instances)
	})
	
	router.POST("/proxy/:id/stop", func(c *gin.Context) {
		instanceID := c.Param("id")
		if err := manager.StopProxy(instanceID); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"status": "stopped"})
	})
	
	router.GET("/status", func(c *gin.Context) {
		hostname, _ := os.Hostname()
		nodeInfo := models.NodeInfo{
			NodeID:   hostname,
			Hostname: hostname,
			Proxies:  manager.GetInstances(),
			UpdatedAt: time.Now(),
		}
		c.JSON(200, nodeInfo)
	})
	
	return router
}

func reportToCoordinator(manager *proxy.Manager) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	client := &http.Client{Timeout: 10 * time.Second}
	hostname, _ := os.Hostname()
	
	for range ticker.C {
		nodeInfo := models.NodeInfo{
			NodeID:   hostname,
			Hostname: hostname,
			Proxies:  manager.GetInstances(),
			UpdatedAt: time.Now(),
		}
		
		data, err := json.Marshal(nodeInfo)
		if err != nil {
			logger.Errorf("Failed to marshal node info: %v", err)
			continue
		}
		
		resp, err := client.Post(
			fmt.Sprintf("%s/api/nodes/%s", cfg.CoordinatorURL, hostname),
			"application/json",
			bytes.NewReader(data),
		)
		if err != nil {
			logger.Errorf("Failed to report to coordinator: %v", err)
			continue
		}
		resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			logger.Warnf("Coordinator returned status %d", resp.StatusCode)
		}
	}
}