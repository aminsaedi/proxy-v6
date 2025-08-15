package ipscanner

import (
	"fmt"
	"net"
	"strings"
	"time"

	"proxy-v6/pkg/models"
	"github.com/sirupsen/logrus"
)

type Scanner struct {
	logger *logrus.Logger
	excludeInterfaces []string
}

func NewScanner(logger *logrus.Logger, excludeInterfaces []string) *Scanner {
	return &Scanner{
		logger: logger,
		excludeInterfaces: excludeInterfaces,
	}
}

func (s *Scanner) ScanIPv6Addresses() ([]models.IPv6Address, error) {
	var ipv6Addresses []models.IPv6Address
	
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}
	
	for _, iface := range interfaces {
		if s.shouldSkipInterface(iface) {
			continue
		}
		
		addrs, err := iface.Addrs()
		if err != nil {
			s.logger.Warnf("Failed to get addresses for interface %s: %v", iface.Name, err)
			continue
		}
		
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			
			if ipNet.IP.To4() != nil {
				continue
			}
			
			ip := ipNet.IP.To16()
			if ip == nil {
				continue
			}
			
			ipv6Addr := models.IPv6Address{
				IP:        ip,
				Interface: iface.Name,
				IsPublic:  s.isPublicIPv6(ip),
				CreatedAt: time.Now(),
			}
			
			if ipv6Addr.IsPublic {
				ipv6Addresses = append(ipv6Addresses, ipv6Addr)
				s.logger.Infof("Found public IPv6: %s on interface %s", ip.String(), iface.Name)
			}
		}
	}
	
	return ipv6Addresses, nil
}

func (s *Scanner) shouldSkipInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return true
	}
	
	if iface.Flags&net.FlagLoopback != 0 {
		return true
	}
	
	for _, excluded := range s.excludeInterfaces {
		if strings.Contains(iface.Name, excluded) {
			return true
		}
	}
	
	return false
}

func (s *Scanner) isPublicIPv6(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	
	if ip.IsPrivate() {
		return false
	}
	
	if len(ip) == 16 && ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}
	
	if len(ip) == 16 && ip[0] == 0xfe && (ip[1]&0xc0) == 0xc0 {
		return false
	}
	
	if len(ip) == 16 && ip[0] == 0xfc || ip[0] == 0xfd {
		return false
	}
	
	return true
}