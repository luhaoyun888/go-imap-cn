package imapserver

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// canStartTLS 检查当前连接是否可以开始 TLS 协商。
func (c *Conn) canStartTLS() bool {
	_, isTLS := c.conn.(*tls.Conn)                                                                  // 检查当前连接是否已经是 TLS 连接
	return c.server.options.TLSConfig != nil && c.state == imap.ConnStateNotAuthenticated && !isTLS // 确保 TLS 配置存在、状态为未认证并且当前连接不是 TLS
}

// handleStartTLS 处理 STARTTLS 命令。
func (c *Conn) handleStartTLS(tag string, dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() { // 检查命令是否以 CRLF 结束
		return dec.Err() // 返回解码错误
	}

	if c.server.options.TLSConfig == nil { // 如果服务器没有 TLS 配置
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "不支持 STARTTLS",
		}
	}
	if !c.canStartTLS() { // 如果当前连接不能开始 TLS
		return &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "STARTTLS 不可用",
		}
	}

	// 不允许在此之后写入明文数据：在此期间保持 c.encMutex 锁定
	enc := newResponseEncoder(c) // 创建响应编码器
	defer enc.end()              // 确保在函数结束时释放编码器

	err := writeStatusResp(enc.Encoder, tag, &imap.StatusResponse{
		Type: imap.StatusResponseTypeOK,
		Text: "现在开始 TLS 协商",
	})
	if err != nil {
		return err // 返回写入状态响应的错误
	}

	// 从 bufio.Reader 中排空缓冲数据
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, c.br, int64(c.br.Buffered())); err != nil {
		panic(err) // 不可达
	}

	var cleartextConn net.Conn
	if buf.Len() > 0 { // 如果缓冲区有数据
		r := io.MultiReader(&buf, c.conn)       // 将缓冲数据与当前连接合并
		cleartextConn = startTLSConn{c.conn, r} // 创建新的连接
	} else {
		cleartextConn = c.conn // 使用当前连接
	}

	tlsConn := tls.Server(cleartextConn, c.server.options.TLSConfig) // 创建 TLS 连接

	c.mutex.Lock()
	c.conn = tlsConn // 更新连接为 TLS 连接
	c.mutex.Unlock()

	rw := c.server.options.wrapReadWriter(tlsConn) // 包装读写器
	c.br.Reset(rw)                                 // 重置读取器
	c.bw.Reset(rw)                                 // 重置写入器

	return nil // 返回 nil 表示成功
}

// startTLSConn 是一个 net.Conn 的包装，支持在 TLS 协商中使用。
type startTLSConn struct {
	net.Conn           // 嵌入 net.Conn
	r        io.Reader // 读取器
}

// Read 从读取器中读取数据。
func (conn startTLSConn) Read(b []byte) (int, error) {
	return conn.r.Read(b) // 调用读取器的 Read 方法
}
