package service

import (
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const egressDialTimeout = 30 * time.Second

// egressDialer returns a net.Dialer for upstream egress. When blockPrivate is set
// it screens the POST-DNS-resolution remote IP of every connection against
// private/loopback/link-local/metadata ranges. Screening at Dialer.Control (which
// receives the resolved ip:port) defeats DNS-rebinding that a URL-string check
// cannot catch.
func egressDialer(blockPrivate bool) *net.Dialer {
	dialer := &net.Dialer{Timeout: egressDialTimeout, KeepAlive: egressDialTimeout}
	if blockPrivate {
		dialer.Control = screenEgressConn
	}
	return dialer
}

func screenEgressConn(_ string, address string, _ syscall.RawConn) error {
	return screenEgressAddress(address)
}

func screenEgressAddress(address string) error {
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		// Dialer.Control always receives a resolved literal ip:port. A non-IP host
		// here is unexpected; treat it as unsafe rather than silently allowing it.
		return errEgressBlocked
	}
	if blockedEgressIP(ip) {
		return errEgressBlocked
	}
	return nil
}

// blockedEgressIP reports whether dialing ip would reach a non-public network the
// gateway must never be coerced into contacting.
func blockedEgressIP(ip net.IP) bool {
	if ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || // 169.254.0.0/16 (incl. 169.254.169.254 metadata), fe80::/10
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsPrivate() { // RFC1918 10/8, 172.16/12, 192.168/16, and ULA fc00::/7
		return true
	}
	// RFC 6598 carrier-grade NAT 100.64.0.0/10 is not covered by net.IP.IsPrivate.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// errEgressBlocked is returned from the dial Control; it never echoes the blocked
// host/IP so an internal address is not leaked to the caller.
var errEgressBlocked = contract.RuntimeError{
	Class:      "egress_blocked",
	StatusCode: http.StatusBadGateway,
	Message:    "upstream egress to a non-public network address is blocked",
}

// accountHasProxy reports whether the account routes egress through an explicit
// proxy URL. Proxied egress is operator-trusted and left unscreened in v1.
func accountHasProxy(account contract.AccountRuntime) bool {
	return strings.Contains(proxyID(account.ProxyID), "://")
}
