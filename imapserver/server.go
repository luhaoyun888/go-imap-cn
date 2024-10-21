// 包 imapserver 实现了一个 IMAP 服务器。
package imapserver

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/luhaoyun888/go-imap-cn"
)

var errClosed = errors.New("imapserver: 服务器已关闭")

// Logger 是一个记录错误信息的工具。
type Logger interface {
	Printf(format string, args ...interface{})
}

// Options 包含服务器选项。
//
// 唯一必需的字段是 NewSession。
type Options struct {
	// NewSession 在客户端连接时被调用。
	NewSession func(*Conn) (Session, *GreetingData, error)
	// 支持的能力。如果为 nil，则只会广告 IMAP4rev1。该集合必须至少包含 IMAP4rev1 或 IMAP4rev2。
	//
	// 以下能力是 IMAP4rev2 的一部分，需要由仅支持 IMAP4rev1 的服务器显式启用：
	//
	//   - NAMESPACE
	//   - UIDPLUS
	//   - ESEARCH
	//   - LIST-EXTENDED
	//   - LIST-STATUS
	//   - MOVE
	//   - STATUS=SIZE
	Caps imap.CapSet
	// Logger 是用于打印错误消息的记录器。如果为 nil，则使用 log.Default。
	Logger Logger
	// TLSConfig 是用于 STARTTLS 的 TLS 配置。如果为 nil，则禁用 STARTTLS。
	TLSConfig *tls.Config
	// InsecureAuth 允许客户端在没有 TLS 的情况下进行身份验证。在这种模式下，服务器容易受到中间人攻击。
	InsecureAuth bool
	// 原始输入和输出数据将写入此写入器（如果有的话）。
	// 请注意，这可能包含敏感信息，例如身份验证期间使用的凭据。
	DebugWriter io.Writer
}

// wrapReadWriter 包装给定的读写器，如果 DebugWriter 不为 nil，则会将调试信息写入 DebugWriter。
func (options *Options) wrapReadWriter(rw io.ReadWriter) io.ReadWriter {
	if options.DebugWriter == nil {
		return rw
	}
	return struct {
		io.Reader
		io.Writer
	}{
		Reader: io.TeeReader(rw, options.DebugWriter),
		Writer: io.MultiWriter(rw, options.DebugWriter),
	}
}

// caps 返回服务器的能力集。如果未设置 Caps，则默认返回只支持 IMAP4rev1 的能力集。
func (options *Options) caps() imap.CapSet {
	if options.Caps != nil {
		return options.Caps
	}
	return imap.CapSet{imap.CapIMAP4rev1: {}}
}

// Server 是一个 IMAP 服务器。
type Server struct {
	options Options

	listenerWaitGroup sync.WaitGroup

	mutex     sync.Mutex
	listeners map[net.Listener]struct{}
	conns     map[*Conn]struct{}
	closed    bool
}

// New 创建一个新的服务器。
func New(options *Options) *Server {
	if caps := options.caps(); !caps.Has(imap.CapIMAP4rev2) && !caps.Has(imap.CapIMAP4rev1) {
		panic("imapserver: 至少必须支持 IMAP4rev1")
	}
	return &Server{
		options:   *options,
		listeners: make(map[net.Listener]struct{}),
		conns:     make(map[*Conn]struct{}),
	}
}

// logger 返回服务器的记录器，如果未设置 Logger，则返回默认记录器。
func (s *Server) logger() Logger {
	if s.options.Logger == nil {
		return log.Default()
	}
	return s.options.Logger
}

// Serve 接受在监听器 ln 上的传入连接。
func (s *Server) Serve(ln net.Listener) error {
	s.mutex.Lock()
	ok := !s.closed
	if ok {
		s.listeners[ln] = struct{}{}
	}
	s.mutex.Unlock()
	if !ok {
		return errClosed
	}

	defer func() {
		s.mutex.Lock()
		delete(s.listeners, ln)
		s.mutex.Unlock()
	}()

	s.listenerWaitGroup.Add(1)
	defer s.listenerWaitGroup.Done()

	var delay time.Duration
	for {
		conn, err := ln.Accept()
		if ne, ok := err.(net.Error); ok && ne.Temporary() {
			if delay == 0 {
				delay = 5 * time.Millisecond
			} else {
				delay *= 2
			}
			if max := 1 * time.Second; delay > max {
				delay = max
			}
			s.logger().Printf("接受错误（将在 %v 后重试）：%v", delay, err)
			time.Sleep(delay)
			continue
		} else if errors.Is(err, net.ErrClosed) {
			return nil
		} else if err != nil {
			return fmt.Errorf("接受错误：%w", err)
		}

		delay = 0
		go newConn(conn, s).serve()
	}
}

// ListenAndServe 在 TCP 网络地址 addr 上监听，然后调用 Serve。
//
// 如果 addr 为空，则使用 ":143"。
func (s *Server) ListenAndServe(addr string) error {
	if addr == "" {
		addr = ":143"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// ListenAndServeTLS 在 TCP 网络地址 addr 上监听，然后调用
// Serve 以处理传入的 TLS 连接。
//
// 在 Options.TLSConfig 中设置的 TLS 配置将被使用。如果 addr 为空，
// 则使用 ":993"。
func (s *Server) ListenAndServeTLS(addr string) error {
	if addr == "" {
		addr = ":993"
	}
	ln, err := tls.Listen("tcp", addr, s.options.TLSConfig)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Close 立即关闭所有活动的监听器和连接。
//
// Close 返回关闭服务器底层监听器时返回的任何错误。
//
// 一旦对服务器调用 Close，就不能再重用；对 Serve 等方法的未来调用将返回错误。
func (s *Server) Close() error {
	var err error

	s.mutex.Lock()
	ok := !s.closed
	if ok {
		s.closed = true
		for l := range s.listeners {
			if closeErr := l.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
	}
	s.mutex.Unlock()
	if !ok {
		return errClosed
	}

	s.listenerWaitGroup.Wait()

	s.mutex.Lock()
	for c := range s.conns {
		c.mutex.Lock()
		c.conn.Close()
		c.mutex.Unlock()
	}
	s.mutex.Unlock()

	return err
}
