package mitm

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

// Proxy 是 MITM 拦截代理服务器。它监听本地端口，对命中规则的 HTTPS 请求进行拦截（动态签发证书），
// 对未命中的请求透明转发。
type Proxy struct {
	mu           sync.Mutex
	listener     net.Listener
	server       *http.Server
	ca           *CertificateAuthority
	rules        *Rules
	gatewayURL   string // http://127.0.0.1:4318
	interceptLog InterceptLogger
	addr         string
}

// NewProxy 创建 Proxy 实例。addr 为监听地址（如 ":8080"），gatewayURL 为本地网关地址。
func NewProxy(addr string, ca *CertificateAuthority, rules *Rules, gatewayURL string, logger InterceptLogger) *Proxy {
	return &Proxy{
		ca:           ca,
		rules:        rules,
		gatewayURL:   gatewayURL,
		interceptLog: logger,
		addr:         addr,
	}
}

// Start 启动代理服务器。
func (p *Proxy) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.listener != nil {
		return fmt.Errorf("proxy already running")
	}

	listener, err := net.Listen("tcp", p.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", p.addr, err)
	}
	p.listener = listener

	// 直接使用 handler 函数而非 ServeMux——ServeMux 对 CONNECT 方法的路由行为不一致
	p.server = &http.Server{
		Handler:           http.HandlerFunc(p.handleRequest),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		p.mu.Lock()
		srv := p.server
		p.mu.Unlock()
		if srv != nil {
			_ = srv.Serve(listener)
		}
	}()

	return nil
}

// Stop 停止代理服务器。幂等。
func (p *Proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.server == nil {
		return nil
	}

	_ = p.server.Close()
	p.server = nil
	p.listener = nil
	return nil
}

// IsHealthy 通过本地 TCP 探活判断 Proxy 是否在运行。
func (p *Proxy) IsHealthy() bool {
	p.mu.Lock()
	listener := p.listener
	p.mu.Unlock()
	if listener == nil {
		return false
	}
	addr := listener.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// handleRequest 处理所有进入的 HTTP/HTTPS 请求。
func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// handleConnect 处理 HTTPS CONNECT 隧道请求。
// 命中规则的域名：拦截（动态签发证书 + TLS 拦截后转发到 gateway）。
// 未命中的域名：透传（建立 TCP 隧道，双向 io.Copy）。
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	// CONNECT 请求中 r.URL.Host 可能为空，需要从 r.Host 获取
	host := r.URL.Host
	if host == "" {
		host = r.Host
	}
	// 去掉端口号
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if p.rules.Match(host) {
		p.interceptConnect(w, r, host)
		return
	}
	p.passthroughConnect(w, r, host)
}

// interceptConnect 拦截 HTTPS 连接：劫持 TCP 连接 → 包装 TLS（使用动态签发的证书）→ 读取 HTTP 请求 → 转发到 gateway。
// 对配置了证书 pin 的域名跳过拦截，改为透传，避免 MITM 破坏 pin 校验。
func (p *Proxy) interceptConnect(w http.ResponseWriter, r *http.Request, host string) {
	// 若该域名设置了证书 pin，则跳过 MITM 拦截，改为透传，以保留上游证书的完整性
	if len(GetPins(host)) > 0 {
		p.passthroughConnect(w, r, host)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// 回复 200 让客户端开始 TLS 握手
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 使用 CA 动态签发证书
	tlsConfig := &tls.Config{
		GetCertificate: p.ca.GetCertificate,
		NextProtos:     []string{"http/1.1"}, // 降级到 HTTP/1.1，不使用 HTTP/2
	}

	tlsConn := tls.Server(clientConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	defer tlsConn.Close()

	// 在 TLS 连接上读取 HTTP 请求并转发到 gateway
	p.serveInterceptedTLS(tlsConn, host)
}

// serveInterceptedTLS 在已拦截的 TLS 连接上读取 HTTP 请求并转发到本地 gateway。
func (p *Proxy) serveInterceptedTLS(tlsConn net.Conn, originalHost string) {
	bufReader := newBufReader(tlsConn)
	for {
		req, err := http.ReadRequest(bufReader)
		if err != nil {
			return
		}

		// 重写请求目标为本地 gateway
		req.URL.Scheme = "http"
		req.URL.Host = p.gatewayURL[7:] // 去掉 "http://" 前缀

		// 记录拦截日志
		startTime := time.Now()
		intercepted := true

		// 转发到 gateway
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			resp = &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(stringReader(fmt.Sprintf("gateway error: %v", err))),
				Header:     make(http.Header),
				Proto:      "HTTP/1.1",
				ProtoMajor: 1,
				ProtoMinor: 1,
			}
		}

		if p.interceptLog != nil {
			p.interceptLog.Log(InterceptEntry{
				Time:        startTime,
				Method:      req.Method,
				Host:        originalHost,
				Path:        req.URL.Path,
				Status:      resp.StatusCode,
				Intercepted: intercepted,
				DurationMs:  float64(time.Since(startTime).Microseconds()) / 1000.0,
			})
		}

		_ = resp.Write(tlsConn)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}

		// 如果不是 Keep-Alive 则退出
		if req.Close || resp.Close {
			return
		}
	}
}

// passthroughConnect 透传 HTTPS CONNECT 隧道（不拦截）。
// 如果该域名配置了证书 pin，则先与上游建立 TLS 连接并校验 pin，
// 校验失败则关闭连接并记录告警；通过后透传原始 TLS 流量。
func (p *Proxy) passthroughConnect(w http.ResponseWriter, r *http.Request, host string) {
	// CONNECT 请求的 r.Host 包含 host:port
	targetAddr := r.Host
	if targetAddr == "" {
		targetAddr = host + ":443"
	}

	// 若该域名设置了证书 pin，先主动与上游握手以校验 pin
	pinned := len(GetPins(host)) > 0
	var upstreamTLSConn *tls.Conn
	if pinned {
		dialer := &net.Dialer{Timeout: 30 * time.Second}
		rawConn, err := dialer.Dial("tcp", targetAddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		upstreamTLSConn = tls.Client(rawConn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: false,
		})
		if err := upstreamTLSConn.Handshake(); err != nil {
			_ = upstreamTLSConn.Close()
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		state := upstreamTLSConn.ConnectionState()
		if err := CheckPin(host, &state); err != nil {
			logging.Warningf("mitm: certificate pin check failed for %s: %v", host, err)
			_ = upstreamTLSConn.Close()
			http.Error(w, "certificate pin verification failed", http.StatusBadGateway)
			return
		}
		// pin 校验通过，恢复为裸 TCP 隧道：关闭 TLS 层但保留底层 TCP 连接供 io.Copy
		// 这里直接复用已建立的 TLS 连接做双向透传即可
	}

	targetConn, err := net.DialTimeout("tcp", targetAddr, 30*time.Second)
	if err != nil {
		if upstreamTLSConn != nil {
			_ = upstreamTLSConn.Close()
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer targetConn.Close()

	// 若已建立 pin 校验的 TLS 连接，则使用它替换裸 TCP 连接
	if upstreamTLSConn != nil {
		targetConn.Close()
		targetConn = upstreamTLSConn
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 双向 io.Copy 管道，60s 超时防止泄漏
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(targetConn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(clientConn, targetConn)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(60 * time.Second):
	}
}

// handleHTTP 处理明文 HTTP 请求（非 HTTPS）。
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if p.rules.Match(host) {
		p.interceptRequest(host, w, r)
		return
	}
	p.passthroughHTTP(host, w, r)
}

// interceptRequest 拦截命中规则的 HTTP 请求，转发到本地 gateway。
func (p *Proxy) interceptRequest(host string, w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 重写请求目标为本地 gateway
	r.URL.Scheme = "http"
	r.URL.Host = p.gatewayURL[7:]

	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("gateway error: %v", err), http.StatusBadGateway)
		if p.interceptLog != nil {
			p.interceptLog.Log(InterceptEntry{
				Time:        startTime,
				Method:      r.Method,
				Host:        host,
				Path:        r.URL.Path,
				Status:      http.StatusBadGateway,
				Intercepted: true,
				DurationMs:  float64(time.Since(startTime).Microseconds()) / 1000.0,
			})
		}
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)

	if p.interceptLog != nil {
		p.interceptLog.Log(InterceptEntry{
			Time:        startTime,
			Method:      r.Method,
			Host:        host,
			Path:        r.URL.Path,
			Status:      resp.StatusCode,
			Intercepted: true,
			DurationMs:  float64(time.Since(startTime).Microseconds()) / 1000.0,
		})
	}
}

// passthroughHTTP 透传未命中规则的 HTTP 请求到原站。
func (p *Proxy) passthroughHTTP(host string, w http.ResponseWriter, r *http.Request) {
	r.URL.Scheme = "http"
	r.URL.Host = host

	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
