package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientAddressKey struct{}

// ClientIP returns the address resolved at the trusted-proxy boundary. Forwarded
// headers are ignored unless the immediate peer belongs to a configured proxy CIDR.
func ClientIP(request *http.Request) netip.Addr {
	if address, ok := request.Context().Value(clientAddressKey{}).(netip.Addr); ok {
		return address
	}
	return remoteIP(request.RemoteAddr)
}

func trustedProxyMiddleware(cidrs []string) func(http.Handler) http.Handler {
	trusted := make([]netip.Prefix, 0, len(cidrs))
	for _, value := range cidrs {
		if prefix, err := netip.ParsePrefix(value); err == nil {
			trusted = append(trusted, prefix.Masked())
		}
	}
	contains := func(address netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(address) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			client := remoteIP(r.RemoteAddr)
			if client.IsValid() && contains(client) {
				forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
				current := client
				validChain := true
				for index := len(forwarded) - 1; index >= 0; index-- {
					if !contains(current) {
						break
					}
					value := strings.TrimSpace(forwarded[index])
					if value == "" {
						validChain = false
						break
					}
					address, err := netip.ParseAddr(value)
					if err != nil {
						validChain = false
						break
					}
					current = address.Unmap()
				}
				if validChain {
					client = current
				}
			}
			ctx := context.WithValue(r.Context(), clientAddressKey{}, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func remoteIP(value string) netip.Addr {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
	}
	address, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return netip.Addr{}
	}
	return address.Unmap()
}
