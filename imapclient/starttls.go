package imapclient

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
)

// startTLS 发送一个 STARTTLS 命令。
//
// 与其他命令不同，此方法会阻塞，直到命令完成。
func (c *Client) startTLS(config *tls.Config) error {
	upgradeDone := make(chan struct{}) // 创建一个通道，用于表示升级完成
	cmd := &startTLSCommand{
		tlsConfig:   config,
		upgradeDone: upgradeDone,
	}
	enc := c.beginCommand("STARTTLS", cmd) // 开始发送 STARTTLS 命令
	enc.flush()                            // 刷新编码器
	defer enc.end()                        // 结束命令

	// 一旦客户端发出 STARTTLS 命令，必须等到服务器响应并完成 TLS 协商后，才能发出其他命令
	if err := cmd.wait(); err != nil {
		return err // 返回错误
	}

	// 解码器的 goroutine 将调用 Client.upgradeStartTLS
	<-upgradeDone // 等待升级完成信号

	return cmd.tlsConn.Handshake() // 完成 TLS 握手
}

// upgradeStartTLS 在服务器发送 OK 响应后完成 STARTTLS 升级。它在解码器 goroutine 中运行。
func (c *Client) upgradeStartTLS(startTLS *startTLSCommand) {
	defer close(startTLS.upgradeDone) // 关闭升级完成信号

	// 从我们的 bufio.Reader 中清空缓冲数据
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, c.br, int64(c.br.Buffered())); err != nil {
		panic(err) // 不会到达这里
	}

	var cleartextConn net.Conn
	if buf.Len() > 0 {
		r := io.MultiReader(&buf, c.conn)       // 创建一个多读取器
		cleartextConn = startTLSConn{c.conn, r} // 使用多读取器创建连接
	} else {
		cleartextConn = c.conn // 使用原始连接
	}

	tlsConn := tls.Client(cleartextConn, startTLS.tlsConfig) // 创建 TLS 客户端连接
	rw := c.options.wrapReadWriter(tlsConn)                  // 包装读取和写入器

	c.br.Reset(rw) // 重置 bufio.Reader
	// 不幸的是，我们无法在这里重用 bufio.Writer，因为它与 Client.StartTLS 有竞争
	c.bw = bufio.NewWriter(rw) // 创建新的 bufio.Writer

	startTLS.tlsConn = tlsConn // 设置 TLS 连接
}

type startTLSCommand struct {
	commandBase
	tlsConfig *tls.Config // TLS 配置

	upgradeDone chan<- struct{} // 升级完成信号通道
	tlsConn     *tls.Conn       // TLS 连接
}

type startTLSConn struct {
	net.Conn
	r io.Reader // 自定义读取器
}

// Read 重写 Read 方法
func (conn startTLSConn) Read(b []byte) (int, error) {
	return conn.r.Read(b) // 从自定义读取器读取数据
}
