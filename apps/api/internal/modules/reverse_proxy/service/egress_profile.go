package service

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

const defaultClientCacheKey = "default"

type egressProfile struct {
	TLSTemplate       string
	HTTPVersionPolicy string
	UserAgent         string
	ExtraHeaders      http.Header
	ForbiddenHeaders  map[string]struct{}
}

func resolveEgressProfile(account contract.AccountRuntime) (egressProfile, error) {
	nested := mapValue(account.Metadata["egress_profile"])
	tlsTemplate := normalizeEgressToken(egressString(nested, account.Metadata, "tls_template", "egress_tls_template"))
	if tlsTemplate == "none" || tlsTemplate == "default" {
		tlsTemplate = ""
	}
	profile := egressProfile{
		TLSTemplate:       tlsTemplate,
		HTTPVersionPolicy: normalizeEgressToken(egressString(nested, account.Metadata, "http_version_policy", "egress_http_version_policy")),
		UserAgent:         cleanEgressUserAgent(egressString(nested, account.Metadata, "user_agent", "egress_user_agent")),
	}
	if profile.HTTPVersionPolicy == "" {
		profile.HTTPVersionPolicy = "prefer_h2"
	}
	if _, ok := clientHelloIDForTLSTemplate(profile.TLSTemplate); !ok {
		return egressProfile{}, unsupportedEgressProfile("unsupported TLS egress profile template")
	}
	if err := validateHTTPVersionPolicy(profile); err != nil {
		return egressProfile{}, err
	}
	headers, err := resolveEgressStaticHeaders(nested, account.Metadata)
	if err != nil {
		return egressProfile{}, err
	}
	profile.ExtraHeaders = headers
	forbidden, err := resolveForbiddenEgressHeaders(nested, account.Metadata)
	if err != nil {
		return egressProfile{}, err
	}
	profile.ForbiddenHeaders = forbidden
	if err := rejectUnsupportedEgressFields(nested, account.Metadata); err != nil {
		return egressProfile{}, err
	}
	return profile, nil
}

func validateHTTPVersionPolicy(profile egressProfile) error {
	switch profile.HTTPVersionPolicy {
	case "", "auto", "prefer_h2", "prefer_http2", "prefer_h1", "prefer_http1", "require_h1", "require_http1":
		return nil
	case "require_h2", "require_http2":
		return unsupportedEgressProfile("require_h2 egress profiles need HTTP/2 fingerprint support")
	default:
		return unsupportedEgressProfile("unsupported HTTP version egress profile policy")
	}
}

func validateEgressTargetURL(rawURL string, profile egressProfile) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" {
		return contract.RuntimeError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy target url is invalid"}
	}
	if profile.TLSTemplate == "" {
		return nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "wss":
		return nil
	default:
		return unsupportedEgressProfile("TLS egress profile requires an HTTPS or WSS upstream")
	}
}

func configureTransportForEgress(transport *http.Transport, account contract.AccountRuntime, profile egressProfile) error {
	if profile.requiresHTTP1() {
		disableHTTP2(transport)
	}
	if profile.TLSTemplate == "" {
		return nil
	}
	clientHelloID, ok := clientHelloIDForTLSTemplate(profile.TLSTemplate)
	if !ok {
		return unsupportedEgressProfile("unsupported TLS egress profile template")
	}
	proxyURL, err := parsedProxyURL(account.ProxyID)
	if err != nil {
		return err
	}
	if proxyURL != nil && !strings.EqualFold(proxyURL.Scheme, "http") {
		return unsupportedEgressProfile("TLS egress profile currently supports direct or HTTP CONNECT proxy egress")
	}
	disableHTTP2(transport)
	transport.Proxy = nil
	tlsConfig := transport.TLSClientConfig
	transport.DialTLSContext = func(ctx context.Context, network string, addr string) (net.Conn, error) {
		if proxyURL != nil {
			return dialUTLSHTTP1ViaHTTPProxy(ctx, network, addr, proxyURL, clientHelloID, tlsConfig)
		}
		return dialUTLSHTTP1(ctx, network, addr, clientHelloID, tlsConfig)
	}
	return nil
}

func disableHTTP2(transport *http.Transport) {
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *stdtls.Conn) http.RoundTripper{}
}

func dialUTLSHTTP1(ctx context.Context, network string, addr string, clientHelloID utls.ClientHelloID, tlsConfig *stdtls.Config) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return performUTLSHTTP1Handshake(ctx, rawConn, addr, clientHelloID, tlsConfig)
}

func dialUTLSHTTP1ViaHTTPProxy(ctx context.Context, network string, addr string, proxyURL *url.URL, clientHelloID utls.ClientHelloID, tlsConfig *stdtls.Config) (net.Conn, error) {
	proxyAddr := proxyAddress(proxyURL)
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	rawConn, err := dialer.DialContext(ctx, network, proxyAddr)
	if err != nil {
		return nil, err
	}
	if err := writeHTTPProxyConnect(rawConn, proxyURL, addr); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return performUTLSHTTP1Handshake(ctx, rawConn, addr, clientHelloID, tlsConfig)
}

func performUTLSHTTP1Handshake(ctx context.Context, rawConn net.Conn, addr string, clientHelloID utls.ClientHelloID, tlsConfig *stdtls.Config) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	spec, err := clientHelloSpecForHTTP1(clientHelloID)
	if err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	config := utlsConfigForHTTP1(host, tlsConfig)
	tlsConn := utls.UClient(rawConn, config, utls.HelloCustom)
	if err := tlsConn.ApplyPreset(spec); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("apply uTLS preset: %w", err)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("uTLS handshake: %w", err)
	}
	return tlsConn, nil
}

func utlsConfigForHTTP1(serverName string, tlsConfig *stdtls.Config) *utls.Config {
	config := &utls.Config{ServerName: serverName, NextProtos: []string{"http/1.1"}}
	if tlsConfig == nil {
		return config
	}
	config.RootCAs = tlsConfig.RootCAs
	config.InsecureSkipVerify = tlsConfig.InsecureSkipVerify
	config.VerifyPeerCertificate = tlsConfig.VerifyPeerCertificate
	config.VerifyConnection = func(state utls.ConnectionState) error {
		if tlsConfig.VerifyConnection == nil {
			return nil
		}
		return tlsConfig.VerifyConnection(stdtls.ConnectionState{
			Version:                     state.Version,
			HandshakeComplete:           state.HandshakeComplete,
			DidResume:                   state.DidResume,
			CipherSuite:                 state.CipherSuite,
			NegotiatedProtocol:          state.NegotiatedProtocol,
			NegotiatedProtocolIsMutual:  state.NegotiatedProtocolIsMutual,
			ServerName:                  state.ServerName,
			PeerCertificates:            state.PeerCertificates,
			VerifiedChains:              state.VerifiedChains,
			SignedCertificateTimestamps: state.SignedCertificateTimestamps,
			OCSPResponse:                state.OCSPResponse,
			TLSUnique:                   state.TLSUnique,
		})
	}
	if tlsConfig.ServerName != "" {
		config.ServerName = tlsConfig.ServerName
	}
	if len(tlsConfig.Certificates) > 0 {
		config.Certificates = make([]utls.Certificate, 0, len(tlsConfig.Certificates))
		for _, certificate := range tlsConfig.Certificates {
			config.Certificates = append(config.Certificates, utls.Certificate{
				Certificate:                  certificate.Certificate,
				PrivateKey:                   certificate.PrivateKey,
				OCSPStaple:                   certificate.OCSPStaple,
				SignedCertificateTimestamps:  certificate.SignedCertificateTimestamps,
				Leaf:                         certificate.Leaf,
				SupportedSignatureAlgorithms: toUTLSSignatureSchemes(certificate.SupportedSignatureAlgorithms),
			})
		}
	}
	return config
}

func toUTLSSignatureSchemes(values []stdtls.SignatureScheme) []utls.SignatureScheme {
	if len(values) == 0 {
		return nil
	}
	out := make([]utls.SignatureScheme, 0, len(values))
	for _, value := range values {
		out = append(out, utls.SignatureScheme(value))
	}
	return out
}

func writeHTTPProxyConnect(conn net.Conn, proxyURL *url.URL, targetAddr string) error {
	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddr},
		Host:   targetAddr,
		Header: http.Header{},
	}
	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	}
	if err := request.Write(conn); err != nil {
		return fmt.Errorf("write proxy CONNECT request: %w", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), request)
	if err != nil {
		return fmt.Errorf("read proxy CONNECT response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return fmt.Errorf("proxy CONNECT failed: %s", response.Status)
	}
	return nil
}

func clientHelloSpecForHTTP1(clientHelloID utls.ClientHelloID) (*utls.ClientHelloSpec, error) {
	spec, err := utls.UTLSIdToSpec(clientHelloID)
	if err != nil {
		return nil, err
	}
	for _, extension := range spec.Extensions {
		if alpn, ok := extension.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{"http/1.1"}
			break
		}
	}
	return &spec, nil
}

func clientHelloIDForTLSTemplate(template string) (utls.ClientHelloID, bool) {
	switch normalizeEgressToken(template) {
	case "", "none", "default":
		return utls.ClientHelloID{}, true
	case "chrome", "chrome_auto":
		return utls.HelloChrome_Auto, true
	case "chrome_120":
		return utls.HelloChrome_120, true
	case "chrome_133":
		return utls.HelloChrome_133, true
	case "firefox", "firefox_auto":
		return utls.HelloFirefox_Auto, true
	case "firefox_120":
		return utls.HelloFirefox_120, true
	case "safari", "safari_auto":
		return utls.HelloSafari_Auto, true
	case "safari_16", "safari_16_0":
		return utls.HelloSafari_16_0, true
	case "ios", "ios_auto":
		return utls.HelloIOS_Auto, true
	case "ios_14":
		return utls.HelloIOS_14, true
	case "android_11_okhttp", "android_okhttp_11":
		return utls.HelloAndroid_11_OkHttp, true
	case "randomized":
		return utls.HelloRandomized, true
	case "randomized_alpn":
		return utls.HelloRandomizedALPN, true
	case "randomized_no_alpn":
		return utls.HelloRandomizedNoALPN, true
	default:
		return utls.ClientHelloID{}, false
	}
}

func resolveEgressStaticHeaders(nested map[string]any, metadata map[string]any) (http.Header, error) {
	headers := http.Header{}
	for _, value := range []any{
		egressValue(nested, metadata, "header_set_template", "egress_header_set_template"),
		egressValue(nested, metadata, "extra_static_headers", "egress_extra_static_headers"),
	} {
		if value == nil {
			continue
		}
		parsed, err := parseEgressHeaderMap(value)
		if err != nil {
			return nil, err
		}
		mergeHeaders(headers, parsed)
	}
	for _, entry := range []struct {
		key        string
		headerName string
	}{
		{key: "accept_language", headerName: "Accept-Language"},
		{key: "sec_ch_ua_template", headerName: "Sec-CH-UA"},
		{key: "sec_ch_ua", headerName: "Sec-CH-UA"},
	} {
		if value := egressString(nested, metadata, entry.key, "egress_"+entry.key); value != "" {
			headers.Set(entry.headerName, value)
		}
	}
	if value := egressString(nested, metadata, "accept_encoding", "egress_accept_encoding"); value != "" {
		if !strings.EqualFold(strings.TrimSpace(value), "identity") {
			return nil, unsupportedEgressProfile("accept_encoding egress profile requires response decompression support")
		}
		headers.Set("Accept-Encoding", "identity")
	}
	for key, values := range headers {
		if err := validateStaticEgressHeader(key, values); err != nil {
			return nil, err
		}
	}
	return headers, nil
}

func parseEgressHeaderMap(value any) (http.Header, error) {
	headers := http.Header{}
	switch typed := value.(type) {
	case map[string]any:
		for key, raw := range typed {
			values := egressHeaderValues(raw)
			if len(values) == 0 {
				continue
			}
			headers[http.CanonicalHeaderKey(strings.TrimSpace(key))] = values
		}
	case map[string]string:
		for key, value := range typed {
			if strings.TrimSpace(value) != "" {
				headers.Set(key, strings.TrimSpace(value))
			}
		}
	default:
		return nil, unsupportedEgressProfile("egress profile static headers must be an object")
	}
	return headers, nil
}

func egressHeaderValues(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := cleanHeaderValue(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := cleanHeaderValue(value); text != "" {
			return []string{text}
		}
		return nil
	}
}

func cleanHeaderValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(typed))
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(fmt.Sprint(value)))
	}
}

func validateStaticEgressHeader(key string, values []string) error {
	lower := strings.ToLower(http.CanonicalHeaderKey(strings.TrimSpace(key)))
	if lower == "" {
		return unsupportedEgressProfile("egress profile static header is not allowed")
	}
	switch lower {
	case "host", "content-length", "authorization", "cookie", "user-agent":
		return unsupportedEgressProfile("egress profile static header is not allowed")
	case "accept-encoding":
		for _, value := range values {
			if !strings.EqualFold(strings.TrimSpace(value), "identity") {
				return unsupportedEgressProfile("accept_encoding egress profile requires response decompression support")
			}
		}
	}
	if forbiddenHeader(key, values) {
		return unsupportedEgressProfile("egress profile static header is not allowed")
	}
	return nil
}

func applyEgressStaticHeaders(headers http.Header, profile egressProfile) {
	if len(profile.ExtraHeaders) == 0 {
		return
	}
	keys := make([]string, 0, len(profile.ExtraHeaders))
	for key := range profile.ExtraHeaders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if headers.Get(key) != "" {
			continue
		}
		for _, value := range profile.ExtraHeaders.Values(key) {
			headers.Add(key, value)
		}
	}
}

func resolveForbiddenEgressHeaders(nested map[string]any, metadata map[string]any) (map[string]struct{}, error) {
	value := egressValue(nested, metadata, "forbidden_headers", "egress_forbidden_headers")
	values, err := stringSliceValue(value)
	if err != nil {
		return nil, unsupportedEgressProfile("egress profile forbidden_headers must be a string array")
	}
	out := map[string]struct{}{}
	for _, value := range values {
		key := strings.ToLower(http.CanonicalHeaderKey(strings.TrimSpace(value)))
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out, nil
}

func rejectUnsupportedEgressFields(nested map[string]any, metadata map[string]any) error {
	for _, key := range []string{
		"http2_template",
		"header_order_template",
		"behavior_pacer",
		"challenge_strategy",
		"stream_format",
	} {
		if !emptyEgressValue(egressValue(nested, metadata, key, "egress_"+key)) {
			return unsupportedEgressProfile("egress profile field is not supported yet")
		}
	}
	bodyEncoding := normalizeEgressToken(egressString(nested, metadata, "body_encoding", "egress_body_encoding"))
	if bodyEncoding != "" && bodyEncoding != "identity" {
		return unsupportedEgressProfile("egress profile body_encoding is not supported yet")
	}
	return nil
}

func clientCacheKey(account contract.AccountRuntime) (string, error) {
	profile, err := resolveEgressProfile(account)
	if err != nil {
		return "", err
	}
	proxyKey := proxyID(account.ProxyID)
	transportKey := profile.transportCacheKey()
	if account.AccountID <= 0 && proxyKey == "" && transportKey == "" {
		return defaultClientCacheKey, nil
	}
	parts := []string{
		"account=" + strconv.Itoa(account.AccountID),
		"proxy=" + proxyKey,
		transportKey,
	}
	return strings.Join(parts, "\x00"), nil
}

func (profile egressProfile) transportCacheKey() string {
	parts := []string{}
	if profile.TLSTemplate != "" {
		parts = append(parts, "tls="+profile.TLSTemplate)
	}
	if profile.requiresHTTP1() {
		parts = append(parts, "http="+profile.HTTPVersionPolicy)
	}
	return strings.Join(parts, "\x00")
}

func (profile egressProfile) requiresHTTP1() bool {
	switch profile.HTTPVersionPolicy {
	case "prefer_h1", "prefer_http1", "require_h1", "require_http1":
		return true
	default:
		return profile.TLSTemplate != ""
	}
}

func (profile egressProfile) forbidsHeader(key string) bool {
	if len(profile.ForbiddenHeaders) == 0 {
		return false
	}
	_, ok := profile.ForbiddenHeaders[strings.ToLower(http.CanonicalHeaderKey(strings.TrimSpace(key)))]
	return ok
}

func egressString(nested map[string]any, metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := credentialString(nested, key); value != "" {
			return value
		}
	}
	for _, key := range keys {
		if value := credentialString(metadata, key); value != "" {
			return value
		}
	}
	return ""
}

func egressValue(nested map[string]any, metadata map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := nested[key]; ok {
			return value
		}
	}
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			return value
		}
	}
	return nil
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		return nil
	}
}

func stringSliceValue(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("not a string")
			}
			out = append(out, text)
		}
		return out, nil
	case []string:
		return append([]string(nil), typed...), nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		return []string{typed}, nil
	default:
		return nil, fmt.Errorf("not a string array")
	}
}

func emptyEgressValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case bool:
		return !typed
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	case map[string]string:
		return len(typed) == 0
	default:
		return false
	}
}

func mergeHeaders(target http.Header, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func cleanEgressUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "srapi/") {
		return ""
	}
	return value
}

func normalizeEgressToken(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"))
}

func proxyID(proxyID *string) string {
	if proxyID == nil {
		return ""
	}
	return strings.TrimSpace(*proxyID)
}

func parsedProxyURL(proxyValue *string) (*url.URL, error) {
	raw := proxyID(proxyValue)
	if raw == "" || !strings.Contains(raw, "://") {
		return nil, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return nil, contract.RuntimeError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "reverse proxy proxy url is invalid"}
	}
	return parsed, nil
}

func proxyAddress(proxyURL *url.URL) string {
	if proxyURL.Port() != "" {
		return proxyURL.Host
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "https":
		return net.JoinHostPort(proxyURL.Hostname(), "443")
	default:
		return net.JoinHostPort(proxyURL.Hostname(), "80")
	}
}

func unsupportedEgressProfile(message string) contract.RuntimeError {
	if strings.TrimSpace(message) == "" {
		message = "unsupported egress profile"
	}
	return contract.RuntimeError{Class: "unsupported_egress_profile", StatusCode: http.StatusBadRequest, Message: message}
}
