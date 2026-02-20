package e2e_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type localProxyClient struct{}

func newLocalProxyClient() *localProxyClient {
	return &localProxyClient{}
}

func (*localProxyClient) CreateProxy(_ string, listen, upstream string) (*localProxy, error) {
	if listen == "" {
		listen = "127.0.0.1:0"
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}

	proxy := &localProxy{
		Listen:      ln.Addr().String(),
		upstream:    upstream,
		listener:    ln,
		closed:      make(chan struct{}),
		connections: make(map[net.Conn]struct{}),
	}

	proxy.wg.Add(1)
	go proxy.serve()

	return proxy, nil
}

type localProxy struct {
	Listen   string
	upstream string
	listener net.Listener

	mu     sync.RWMutex
	config localProxyConfig

	connMu      sync.Mutex
	connections map[net.Conn]struct{}

	closeOnce sync.Once
	closed    chan struct{}
	wg        sync.WaitGroup
}

type localProxyConfig struct {
	upstreamRateBytesPerSecond int64
	upstreamLimitBytes         int64
}

type localToxic struct{}

var errUpstreamLimitReached = errors.New("upstream limit reached")

func (proxy *localProxy) AddToxic(_ string, toxicType, stream string, _ float32, attributes map[string]any) (*localToxic, error) {
	if stream != "upstream" {
		return nil, fmt.Errorf("unsupported stream %q", stream)
	}

	proxy.mu.Lock()
	defer proxy.mu.Unlock()

	switch toxicType {
	case "bandwidth":
		rateKBPerSecond, err := int64Attribute(attributes, "rate")
		if err != nil {
			return nil, err
		}
		if rateKBPerSecond <= 0 {
			return nil, fmt.Errorf("bandwidth toxic requires rate > 0")
		}
		proxy.config.upstreamRateBytesPerSecond = rateKBPerSecond * 1024

	case "limit_data":
		limitBytes, err := int64Attribute(attributes, "bytes")
		if err != nil {
			return nil, err
		}
		if limitBytes <= 0 {
			return nil, fmt.Errorf("limit_data toxic requires bytes > 0")
		}
		proxy.config.upstreamLimitBytes = limitBytes

	default:
		return nil, fmt.Errorf("unsupported toxic type %q", toxicType)
	}

	return &localToxic{}, nil
}

func (proxy *localProxy) Delete() error {
	var closeErr error

	proxy.closeOnce.Do(func() {
		close(proxy.closed)
		closeErr = proxy.listener.Close()

		proxy.connMu.Lock()
		for conn := range proxy.connections {
			_ = conn.Close()
		}
		proxy.connMu.Unlock()

		proxy.wg.Wait()
	})

	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func (proxy *localProxy) serve() {
	defer proxy.wg.Done()

	for {
		clientConn, err := proxy.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			select {
			case <-proxy.closed:
				return
			default:
			}

			continue
		}

		proxy.trackConnection(clientConn)
		proxy.wg.Add(1)
		go proxy.handleConnection(clientConn)
	}
}

func (proxy *localProxy) handleConnection(clientConn net.Conn) {
	defer proxy.wg.Done()
	defer proxy.untrackConnection(clientConn)

	upstreamConn, err := net.Dial("tcp", proxy.upstream)
	if err != nil {
		_ = clientConn.Close()
		return
	}

	proxy.trackConnection(upstreamConn)
	defer proxy.untrackConnection(upstreamConn)

	config := proxy.currentConfig()
	errCh := make(chan error, 2)

	go func() {
		errCh <- proxy.copyUpstream(upstreamConn, clientConn, config)
	}()

	go func() {
		_, err := io.Copy(clientConn, upstreamConn)
		errCh <- err
	}()

	firstErr := <-errCh

	_ = clientConn.Close()
	_ = upstreamConn.Close()
	<-errCh

	if errors.Is(firstErr, errUpstreamLimitReached) {
		return
	}
}

func (proxy *localProxy) copyUpstream(dst net.Conn, src net.Conn, config localProxyConfig) error {
	const maxBufferSize = 32 * 1024
	bufferSize := maxBufferSize
	if config.upstreamRateBytesPerSecond > 0 {
		// Keep chunks small enough to avoid large bursts (about 50ms worth of data).
		bufferSize = int(config.upstreamRateBytesPerSecond / 20)
		if bufferSize < 1 {
			bufferSize = 1
		}
		if bufferSize > maxBufferSize {
			bufferSize = maxBufferSize
		}
	}

	buffer := make([]byte, bufferSize)
	sent := int64(0)
	start := time.Now()

	for {
		readLimit := len(buffer)
		if config.upstreamLimitBytes > 0 {
			remaining := config.upstreamLimitBytes - sent
			if remaining <= 0 {
				return errUpstreamLimitReached
			}
			if int64(readLimit) > remaining {
				readLimit = int(remaining)
			}
		}

		readLen, readErr := src.Read(buffer[:readLimit])
		if readLen > 0 {
			if config.upstreamRateBytesPerSecond > 0 {
				expectedElapsed := time.Duration(sent+int64(readLen)) * time.Second / time.Duration(config.upstreamRateBytesPerSecond)
				if sleepFor := time.Until(start.Add(expectedElapsed)); sleepFor > 0 {
					time.Sleep(sleepFor)
				}
			}

			if err := writeAll(dst, buffer[:readLen]); err != nil {
				return err
			}
			sent += int64(readLen)

			if config.upstreamLimitBytes > 0 && sent >= config.upstreamLimitBytes {
				return errUpstreamLimitReached
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func (proxy *localProxy) currentConfig() localProxyConfig {
	proxy.mu.RLock()
	defer proxy.mu.RUnlock()

	return proxy.config
}

func (proxy *localProxy) trackConnection(conn net.Conn) {
	proxy.connMu.Lock()
	defer proxy.connMu.Unlock()

	proxy.connections[conn] = struct{}{}
}

func (proxy *localProxy) untrackConnection(conn net.Conn) {
	proxy.connMu.Lock()
	defer proxy.connMu.Unlock()

	delete(proxy.connections, conn)
	_ = conn.Close()
}

func int64Attribute(attributes map[string]any, key string) (int64, error) {
	rawValue, ok := attributes[key]
	if !ok {
		return 0, fmt.Errorf("missing %q attribute", key)
	}

	switch value := rawValue.(type) {
	case int:
		return int64(value), nil
	case int8:
		return int64(value), nil
	case int16:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case uint:
		return int64(value), nil
	case uint8:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint32:
		return int64(value), nil
	case uint64:
		return int64(value), nil
	case float32:
		return floatAttributeToInt64(float64(value), key)
	case float64:
		return floatAttributeToInt64(value, key)
	default:
		return 0, fmt.Errorf("attribute %q has unsupported type %T", key, rawValue)
	}
}

func floatAttributeToInt64(value float64, key string) (int64, error) {
	intValue := int64(value)
	if float64(intValue) != value {
		return 0, fmt.Errorf("attribute %q must be an integer", key)
	}

	return intValue, nil
}

func writeAll(dst io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := dst.Write(data)
		if err != nil {
			return err
		}
		data = data[written:]
	}

	return nil
}
