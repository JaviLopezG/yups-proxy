package metadata

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

var privateIPBlocks []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",        // IPv4 loopback
		"10.0.0.0/8",         // RFC1918
		"172.16.0.0/12",      // RFC1918
		"192.168.0.0/16",     // RFC1918
		"169.254.0.0/16",     // RFC3927 link-local & cloud metadata (169.254.169.254)
		"100.64.0.0/10",      // CGNAT (RFC 6598)
		"192.0.0.0/24",       // IETF Protocol Assignments
		"192.0.2.0/24",       // TEST-NET-1
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"224.0.0.0/4",        // Multicast
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
	}

	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err == nil {
			privateIPBlocks = append(privateIPBlocks, block)
		}
	}
}

// IsPublicIP reports whether ip is a routable, non-private public IP address.
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// Normalize IPv4 if it was parsed as 16-byte IPv4-mapped IPv6
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}

	// Check against known private/reserved CIDRs
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// NewSafeTransport returns an http.Transport that prevents SSRF attacks
// by validating resolved IPs before initiating TCP connections.
func NewSafeTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   2 * time.Second,
		KeepAlive: 10 * time.Second,
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", addr, err)
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dns resolution failed for %q: %w", host, err)
			}

			var publicIP net.IP
			for _, ipAddr := range ips {
				if IsPublicIP(ipAddr.IP) {
					publicIP = ipAddr.IP
					break
				}
			}

			if publicIP == nil {
				return nil, fmt.Errorf("ssrf blocked: host %q resolved to private/blocked ip", host)
			}

			target := net.JoinHostPort(publicIP.String(), port)
			return dialer.DialContext(ctx, network, target)
		},
		ResponseHeaderTimeout: 3 * time.Second,
		DisableKeepAlives:     true,
	}
}

// NewSafeHTTPClient creates an http.Client configured with SSRF protection and a strict timeout.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &http.Client{
		Transport: NewSafeTransport(),
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("stopped after 3 redirects")
			}
			return nil
		},
	}
}
