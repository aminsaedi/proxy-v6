package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"proxy-v6/internal/loadbalancer"
	"proxy-v6/pkg/models"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logger *logrus.Logger
	cfg    models.CoordinatorConfig
	nodes  map[string]models.NodeInfo
	mu     sync.RWMutex
)

func main() {
	logger = logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	nodes = make(map[string]models.NodeInfo)
	
	rootCmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Coordinator service for managing distributed IPv6 proxies",
		Run:   runCoordinator,
	}
	
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
	rootCmd.PersistentFlags().IntP("port", "p", 8081, "API listen port")
	rootCmd.PersistentFlags().IntP("proxy-port", "", 8888, "Proxy listen port")
	rootCmd.PersistentFlags().IntP("metrics-port", "m", 9091, "Metrics port")
	rootCmd.PersistentFlags().DurationP("health-interval", "", 30*time.Second, "Health check interval")
	
	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		logger.Fatalf("Failed to bind flags: %v", err)
	}
	
	if err := rootCmd.Execute(); err != nil {
		logger.Fatal(err)
	}
}

func runCoordinator(cmd *cobra.Command, args []string) {
	configFile := viper.GetString("config")
	if configFile != "" {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			logger.Warnf("Failed to read config file: %v", err)
		}
	}
	
	cfg = models.CoordinatorConfig{
		ListenPort:          viper.GetInt("port"),
		ProxyPort:           viper.GetInt("proxy-port"),
		MetricsPort:         viper.GetInt("metrics-port"),
		HealthCheckInterval: viper.GetDuration("health-interval"),
	}
	
	lb := loadbalancer.NewLoadBalancer(logger, cfg.HealthCheckInterval)
	
	router := setupAPIRouter(lb)
	
	go func() {
		metricsRouter := gin.New()
		metricsRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))
		logger.Infof("Starting metrics server on port %d", cfg.MetricsPort)
		if err := metricsRouter.Run(fmt.Sprintf(":%d", cfg.MetricsPort)); err != nil {
			logger.Errorf("Metrics server error: %v", err)
		}
	}()
	
	go startProxyServer(lb)
	
	go cleanupStaleNodes()
	
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("Server shutdown error: %v", err)
	}
}

func setupAPIRouter(lb *loadbalancer.LoadBalancer) *gin.Engine {
	router := gin.Default()
	
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})
	
	router.POST("/api/nodes/:nodeId", func(c *gin.Context) {
		nodeID := c.Param("nodeId")
		
		var nodeInfo models.NodeInfo
		if err := c.ShouldBindJSON(&nodeInfo); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		
		mu.Lock()
		nodes[nodeID] = nodeInfo
		mu.Unlock()
		
		updateLoadBalancer(lb)
		
		c.JSON(200, gin.H{"status": "updated"})
	})
	
	router.GET("/api/nodes", func(c *gin.Context) {
		mu.RLock()
		defer mu.RUnlock()
		
		nodeList := make([]models.NodeInfo, 0, len(nodes))
		for _, node := range nodes {
			nodeList = append(nodeList, node)
		}
		
		c.JSON(200, nodeList)
	})
	
	router.GET("/api/stats", func(c *gin.Context) {
		mu.RLock()
		defer mu.RUnlock()
		
		totalProxies := 0
		healthyProxies := 0
		
		for _, node := range nodes {
			for _, proxy := range node.Proxies {
				totalProxies++
				if proxy.Status == models.ProxyStatusRunning {
					healthyProxies++
				}
			}
		}
		
		stats := gin.H{
			"total_nodes":     len(nodes),
			"total_proxies":   totalProxies,
			"healthy_proxies": healthyProxies,
			"timestamp":       time.Now(),
		}
		
		c.JSON(200, stats)
	})
	
	return router
}

func startProxyServer(lb *loadbalancer.LoadBalancer) {
	logger.Infof("Starting proxy server on port %d", cfg.ProxyPort)
	
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ProxyPort),
		Handler:      lb,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("Proxy server error: %v", err)
	}
}

func updateLoadBalancer(lb *loadbalancer.LoadBalancer) {
	mu.RLock()
	defer mu.RUnlock()
	
	nodeList := make([]models.NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}
	
	lb.UpdateProxies(nodeList)
}

func cleanupStaleNodes() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		mu.Lock()
		now := time.Now()
		for nodeID, node := range nodes {
			if now.Sub(node.UpdatedAt) > 2*time.Minute {
				logger.Warnf("Removing stale node: %s", nodeID)
				delete(nodes, nodeID)
			}
		}
		mu.Unlock()
	}
}