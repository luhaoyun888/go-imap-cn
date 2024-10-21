package imapserver

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

const (
	cmdReadTimeout     = 30 * time.Second
	idleReadTimeout    = 35 * time.Minute // 第 5.4 节规定最少 30 分钟
	literalReadTimeout = 5 * time.Minute

	respWriteTimeout    = 30 * time.Second
	literalWriteTimeout = 5 * time.Minute
)

var internalServerErrorResp = &imap.StatusResponse{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeServerBug,
	Text: "内部服务器错误",
}

// Conn 代表与 IMAP 服务器的连接。
type Conn struct {
	server   *Server       // 服务器实例
	br       *bufio.Reader // 输入缓冲区
	bw       *bufio.Writer // 输出缓冲区
	encMutex sync.Mutex    // 编码器的互斥锁

	mutex   sync.Mutex  // 连接的互斥锁
	conn    net.Conn    // 网络连接
	enabled imap.CapSet // 启用的能力集

	state   imap.ConnState // 当前连接状态
	session Session        // 当前会话
}

// newConn 创建一个新的 IMAP 连接。
func newConn(c net.Conn, server *Server) *Conn {
	rw := server.options.wrapReadWriter(c) // 包装网络连接以支持读写
	br := bufio.NewReader(rw)              // 创建输入缓冲区
	bw := bufio.NewWriter(rw)              // 创建输出缓冲区
	return &Conn{
		conn:    c,
		server:  server,
		br:      br,
		bw:      bw,
		enabled: make(imap.CapSet), // 初始化能力集
	}
}

// NetConn 返回被 IMAP 连接包装的底层网络连接。
//
// 直接对该连接进行读写操作会破坏 IMAP 会话。
func (c *Conn) NetConn() net.Conn {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.conn // 返回网络连接
}

// Bye 终止 IMAP 连接。
func (c *Conn) Bye(text string) error {
	respErr := c.writeStatusResp("", &imap.StatusResponse{
		Type: imap.StatusResponseTypeBye,
		Text: text,
	})
	closeErr := c.conn.Close() // 关闭连接
	if respErr != nil {
		return respErr
	}
	return closeErr // 返回关闭错误
}

// serve 处理IMAP连接的主要逻辑。
func (c *Conn) serve() {
	defer func() {
		if v := recover(); v != nil {
			c.server.logger().Printf("处理命令时发生panic: %v\n%s", v, debug.Stack())
		}
		c.conn.Close()
	}()

	c.server.mutex.Lock()
	c.server.conns[c] = struct{}{}
	c.server.mutex.Unlock()
	defer func() {
		c.server.mutex.Lock()
		delete(c.server.conns, c)
		c.server.mutex.Unlock()
	}()

	var (
		greetingData *GreetingData // 欢迎信息
		err          error         // 错误信息
	)
	c.session, greetingData, err = c.server.options.NewSession(c)
	if err != nil {
		var (
			resp    *imap.StatusResponse
			imapErr *imap.Error
		)
		if errors.As(err, &imapErr) && imapErr.Type == imap.StatusResponseTypeBye {
			resp = (*imap.StatusResponse)(imapErr)
		} else {
			c.server.logger().Printf("创建会话失败: %v", err)
			resp = internalServerErrorResp
		}
		if err := c.writeStatusResp("", resp); err != nil {
			c.server.logger().Printf("写入欢迎信息失败: %v", err)
		}
		return
	}

	defer func() {
		if c.session != nil {
			if err := c.session.Close(); err != nil {
				c.server.logger().Printf("关闭会话失败: %v", err)
			}
		}
	}()

	caps := c.server.options.caps()
	if _, ok := c.session.(SessionIMAP4rev2); !ok && caps.Has(imap.CapIMAP4rev2) {
		panic("imapserver: 服务器声明支持IMAP4rev2，但会话不支持")
	}
	if _, ok := c.session.(SessionNamespace); !ok && caps.Has(imap.CapNamespace) {
		panic("imapserver: 服务器声明支持NAMESPACE，但会话不支持")
	}
	if _, ok := c.session.(SessionMove); !ok && caps.Has(imap.CapMove) {
		panic("imapserver: 服务器声明支持MOVE，但会话不支持")
	}
	if _, ok := c.session.(SessionUnauthenticate); !ok && caps.Has(imap.CapUnauthenticate) {
		panic("imapserver: 服务器声明支持UNAUTHENTICATE，但会话不支持")
	}

	c.state = imap.ConnStateNotAuthenticated // 初始状态为未认证
	statusType := imap.StatusResponseTypeOK  // 默认状态为OK
	if greetingData != nil && greetingData.PreAuth {
		c.state = imap.ConnStateAuthenticated // 如果支持预认证，则状态为已认证
		statusType = imap.StatusResponseTypePreAuth
	}
	if err := c.writeCapabilityStatus("", statusType, "IMAP 服务器已准备就绪"); err != nil {
		c.server.logger().Printf("写入欢迎信息失败: %v", err)
		return
	}

	for {
		var readTimeout time.Duration
		switch c.state {
		case imap.ConnStateAuthenticated, imap.ConnStateSelected:
			readTimeout = idleReadTimeout // 认证或选择状态下的超时时间
		default:
			readTimeout = cmdReadTimeout // 默认命令读取超时时间
		}
		c.setReadTimeout(readTimeout)

		dec := imapwire.NewDecoder(c.br, imapwire.ConnSideServer) // 创建解码器
		dec.CheckBufferedLiteralFunc = c.checkBufferedLiteral     // 设置缓冲字面量检查

		if c.state == imap.ConnStateLogout || dec.EOF() {
			break // 如果状态为注销或EOF，则退出循环
		}

		c.setReadTimeout(cmdReadTimeout)
		if err := c.readCommand(dec); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				c.server.logger().Printf("读取命令失败: %v", err)
			}
			break
		}
	}
}

// readCommand 读取并解析客户端发送的命令。
func (c *Conn) readCommand(dec *imapwire.Decoder) error {
	var tag, name string
	// 期望解析出命令标签和命令名称
	if !dec.ExpectAtom(&tag) || !dec.ExpectSP() || !dec.ExpectAtom(&name) {
		return fmt.Errorf("在命令中: %w", dec.Err())
	}
	name = strings.ToUpper(name) // 将命令名称转换为大写

	numKind := NumKindSeq // 默认使用序列号
	if name == "UID" {
		numKind = NumKindUID // 如果是UID命令，更新为UID类型
		var subName string
		// 解析UID命令的子名称
		if !dec.ExpectSP() || !dec.ExpectAtom(&subName) {
			return fmt.Errorf("在命令中: %w", dec.Err())
		}
		name = "UID " + strings.ToUpper(subName) // 组合UID命令
	}

	// TODO: 处理多个命令并发执行
	sendOK := true
	var err error
	// 根据命令名称调用相应的处理函数
	switch name {
	case "NOOP", "CHECK":
		err = c.handleNoop(dec)
	case "LOGOUT":
		err = c.handleLogout(dec)
	case "CAPABILITY":
		err = c.handleCapability(dec)
	case "STARTTLS":
		err = c.handleStartTLS(tag, dec)
		sendOK = false // STARTTLS不发送OK响应
	case "AUTHENTICATE":
		err = c.handleAuthenticate(tag, dec)
		sendOK = false
	case "UNAUTHENTICATE":
		err = c.handleUnauthenticate(dec)
	case "LOGIN":
		err = c.handleLogin(tag, dec)
		sendOK = false
	case "ENABLE":
		err = c.handleEnable(dec)
	case "CREATE":
		err = c.handleCreate(dec)
	case "DELETE":
		err = c.handleDelete(dec)
	case "RENAME":
		err = c.handleRename(dec)
	case "SUBSCRIBE":
		err = c.handleSubscribe(dec)
	case "UNSUBSCRIBE":
		err = c.handleUnsubscribe(dec)
	case "STATUS":
		err = c.handleStatus(dec)
	case "LIST":
		err = c.handleList(dec)
	case "LSUB":
		err = c.handleLSub(dec)
	case "NAMESPACE":
		err = c.handleNamespace(dec)
	case "IDLE":
		err = c.handleIdle(dec)
	case "SELECT", "EXAMINE":
		err = c.handleSelect(tag, dec, name == "EXAMINE")
		sendOK = false
	case "CLOSE", "UNSELECT":
		err = c.handleUnselect(dec, name == "CLOSE")
	case "APPEND":
		err = c.handleAppend(tag, dec)
		sendOK = false
	case "FETCH", "UID FETCH":
		err = c.handleFetch(dec, numKind)
	case "EXPUNGE":
		err = c.handleExpunge(dec)
	case "UID EXPUNGE":
		err = c.handleUIDExpunge(dec)
	case "STORE", "UID STORE":
		err = c.handleStore(dec, numKind)
	case "COPY", "UID COPY":
		err = c.handleCopy(tag, dec, numKind)
		sendOK = false
	case "MOVE", "UID MOVE":
		err = c.handleMove(dec, numKind)
	case "SEARCH", "UID SEARCH":
		err = c.handleSearch(tag, dec, numKind)
	default:
		// 处理未识别的命令
		if c.state == imap.ConnStateNotAuthenticated {
			// 在未认证状态下不允许任何未知命令，以防止跨协议攻击
			c.state = imap.ConnStateLogout
			defer c.Bye("命令无法识别")
		}
		err = &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "命令无法识别",
		}
	}

	dec.DiscardLine() // 丢弃解码器中的当前行

	var (
		resp    *imap.StatusResponse
		imapErr *imap.Error
		decErr  *imapwire.DecoderExpectError
	)
	// 根据错误类型构造响应
	if errors.As(err, &imapErr) {
		resp = (*imap.StatusResponse)(imapErr)
	} else if errors.As(err, &decErr) {
		resp = &imap.StatusResponse{
			Type: imap.StatusResponseTypeBad,
			Code: imap.ResponseCodeClientBug,
			Text: "语法错误: " + decErr.Message,
		}
	} else if err != nil {
		c.server.logger().Printf("正在处理 %v 命令: %v", name, err)
		resp = internalServerErrorResp // 处理服务器内部错误
	} else {
		if !sendOK {
			return nil // 如果不需要发送OK响应，直接返回
		}
		if err := c.poll(name); err != nil {
			return err // 处理命令后续的轮询
		}
		resp = &imap.StatusResponse{
			Type: imap.StatusResponseTypeOK,
			Text: fmt.Sprintf("%v 完成", name), // 命令成功完成
		}
	}
	return c.writeStatusResp(tag, resp) // 写入状态响应
}

// handleNoop 处理NOOP命令（无操作）。
func (c *Conn) handleNoop(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 期望CRLF
	}
	return nil
}

// handleLogout 处理LOGOUT命令（注销）。
func (c *Conn) handleLogout(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 期望CRLF
	}

	c.state = imap.ConnStateLogout // 更新连接状态为注销

	return c.writeStatusResp("", &imap.StatusResponse{
		Type: imap.StatusResponseTypeBye,
		Text: "注销", // 返回注销消息
	})
}

// handleDelete 处理DELETE命令（删除邮箱）。
func (c *Conn) handleDelete(dec *imapwire.Decoder) error {
	var name string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&name) || !dec.ExpectCRLF() {
		return dec.Err() // 期望邮箱名称和CRLF
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 检查当前状态是否为已认证
	}
	return c.session.Delete(name) // 删除指定的邮箱
}

// handleRename 处理RENAME命令（重命名邮箱）。
func (c *Conn) handleRename(dec *imapwire.Decoder) error {
	var oldName, newName string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&oldName) || !dec.ExpectSP() || !dec.ExpectMailbox(&newName) || !dec.ExpectCRLF() {
		return dec.Err() // 期望邮箱名称和CRLF
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 检查当前状态是否为已认证
	}
	return c.session.Rename(oldName, newName) // 重命名邮箱
}

// handleSubscribe 处理SUBSCRIBE命令（订阅邮箱）。
func (c *Conn) handleSubscribe(dec *imapwire.Decoder) error {
	var name string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&name) || !dec.ExpectCRLF() {
		return dec.Err() // 期望邮箱名称和CRLF
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 检查当前状态是否为已认证
	}
	return c.session.Subscribe(name) // 订阅指定的邮箱
}

// handleUnsubscribe 处理UNSUBSCRIBE命令（取消订阅邮箱）。
func (c *Conn) handleUnsubscribe(dec *imapwire.Decoder) error {
	var name string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&name) || !dec.ExpectCRLF() {
		return dec.Err() // 期望邮箱名称和CRLF
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 检查当前状态是否为已认证
	}
	return c.session.Unsubscribe(name) // 取消订阅指定的邮箱
}

// checkBufferedLiteral 检查字面量缓冲区。
func (c *Conn) checkBufferedLiteral(size int64, nonSync bool) error {
	if size > 4096 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTooBig,
			Text: "此命令的字面量限制为 4096 字节", // 字面量大小限制
		}
	}

	return c.acceptLiteral(size, nonSync) // 接受字面量
}

// acceptLiteral 接受字面量。
func (c *Conn) acceptLiteral(size int64, nonSync bool) error {
	if nonSync && size > 4096 && !c.server.options.caps().Has(imap.CapLiteralPlus) {
		return &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: "非同步字面量限制为 4096 字节", // 非同步字面量大小限制
		}
	}

	if nonSync {
		return nil
	}

	return c.writeContReq("中文什么意思") // 请求发送字面量数据
}

// canAuth 检查是否可以进行认证。
func (c *Conn) canAuth() bool {
	if c.state != imap.ConnStateNotAuthenticated {
		return false // 如果当前状态不是未认证，返回false
	}
	_, isTLS := c.conn.(*tls.Conn)                // 检查连接是否为TLS
	return isTLS || c.server.options.InsecureAuth // 返回TLS或不安全认证选项
}

// writeStatusResp 写入状态响应。
func (c *Conn) writeStatusResp(tag string, statusResp *imap.StatusResponse) error {
	enc := newResponseEncoder(c)
	defer enc.end()                                      // 结束编码
	return writeStatusResp(enc.Encoder, tag, statusResp) // 写入状态响应
}

// writeContReq 写入继续请求。
func (c *Conn) writeContReq(text string) error {
	enc := newResponseEncoder(c)
	defer enc.end()                        // 结束编码
	return writeContReq(enc.Encoder, text) // 写入继续请求
}

// writeCapabilityStatus 写入能力状态响应。
func (c *Conn) writeCapabilityStatus(tag string, typ imap.StatusResponseType, text string) error {
	enc := newResponseEncoder(c)
	defer enc.end()                                                              // 结束编码
	return writeCapabilityStatus(enc.Encoder, tag, typ, c.availableCaps(), text) // 写入能力状态
}

// checkState 检查当前连接状态。
func (c *Conn) checkState(state imap.ConnState) error {
	if state == imap.ConnStateAuthenticated && c.state == imap.ConnStateSelected {
		return nil // 如果状态匹配，返回nil
	}
	if c.state != state {
		return newClientBugError(fmt.Sprintf("此命令只在 %s 状态下有效", state)) // 返回错误
	}
	return nil
}

// setReadTimeout 设置读取超时时间。
func (c *Conn) setReadTimeout(dur time.Duration) {
	if dur > 0 {
		c.conn.SetReadDeadline(time.Now().Add(dur)) // 设置超时
	} else {
		c.conn.SetReadDeadline(time.Time{}) // 取消超时
	}
}

// setWriteTimeout 设置写入超时时间。
func (c *Conn) setWriteTimeout(dur time.Duration) {
	if dur > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(dur)) // 设置超时
	} else {
		c.conn.SetWriteDeadline(time.Time{}) // 取消超时
	}
}

// poll 轮询状态更新。
func (c *Conn) poll(cmd string) error {
	switch c.state {
	case imap.ConnStateAuthenticated, imap.ConnStateSelected:
		// 当前状态为已认证或已选择，无需处理
	default:
		return nil // 其他状态无需处理
	}

	allowExpunge := true
	switch cmd {
	case "FETCH", "STORE", "SEARCH":
		allowExpunge = false // 在特定命令下不允许EXPUNGE
	}

	w := &UpdateWriter{conn: c, allowExpunge: allowExpunge} // 创建更新写入器
	return c.session.Poll(w, allowExpunge)                  // 轮询状态更新
}

// responseEncoder 用于编码IMAP响应。
type responseEncoder struct {
	*imapwire.Encoder       // 包含IMAP编码器
	conn              *Conn // 连接
}

// newResponseEncoder 创建新的响应编码器。
func newResponseEncoder(conn *Conn) *responseEncoder {
	conn.mutex.Lock()
	quotedUTF8 := conn.enabled.Has(imap.CapIMAP4rev2) || conn.enabled.Has(imap.CapUTF8Accept) // 检查是否支持UTF8
	conn.mutex.Unlock()

	wireEnc := imapwire.NewEncoder(conn.bw, imapwire.ConnSideServer) // 创建新的IMAP编码器
	wireEnc.QuotedUTF8 = quotedUTF8

	conn.encMutex.Lock()                   // 获取编码器互斥锁
	conn.setWriteTimeout(respWriteTimeout) // 设置写入超时时间
	return &responseEncoder{
		Encoder: wireEnc,
		conn:    conn,
	}
}

// end 结束响应编码器。
func (enc *responseEncoder) end() {
	if enc.Encoder == nil {
		panic("imapserver：responseEncoder.end 被调用了两次") // 确保不会重复调用
	}
	enc.Encoder = nil           // 释放编码器
	enc.conn.setWriteTimeout(0) // 取消写入超时
	enc.conn.encMutex.Unlock()  // 释放编码器互斥锁
}

// Literal 返回用于写入字面量的写入器。
func (enc *responseEncoder) Literal(size int64) io.WriteCloser {
	enc.conn.setWriteTimeout(literalWriteTimeout) // 设置字面量写入超时时间
	return literalWriter{
		WriteCloser: enc.Encoder.Literal(size, nil), // 创建字面量写入器
		conn:        enc.conn,
	}
}

// literalWriter 用于写入字面量。
type literalWriter struct {
	io.WriteCloser       // 包含的写入器
	conn           *Conn // 连接
}

// Close 关闭字面量写入器并恢复超时时间。
func (lw literalWriter) Close() error {
	lw.conn.setWriteTimeout(respWriteTimeout) // 恢复写入超时时间
	return lw.WriteCloser.Close()             // 关闭写入器
}

// writeStatusResp 写入状态响应。
func writeStatusResp(enc *imapwire.Encoder, tag string, statusResp *imap.StatusResponse) error {
	if tag == "" {
		tag = "*" // 如果标签为空，设置为星号
	}
	enc.Atom(tag).SP().Atom(string(statusResp.Type)).SP() // 编码标签和状态类型
	if statusResp.Code != "" {
		enc.Atom(fmt.Sprintf("[%v]", statusResp.Code)).SP() // 编码状态代码
	}
	enc.Text(statusResp.Text) // 编码状态文本
	return enc.CRLF()         // 写入结束符
}

// writeCapabilityOK 写入能力确认响应。
func writeCapabilityOK(enc *imapwire.Encoder, tag string, caps []imap.Cap, text string) error {
	return writeCapabilityStatus(enc, tag, imap.StatusResponseTypeOK, caps, text) // 写入能力状态
}

// writeCapabilityStatus 写入能力状态响应。
func writeCapabilityStatus(enc *imapwire.Encoder, tag string, typ imap.StatusResponseType, caps []imap.Cap, text string) error {
	if tag == "" {
		tag = "*" // 如果标签为空，设置为星号
	}

	enc.Atom(tag).SP().Atom(string(typ)).SP().Special('[').Atom("CAPABILITY") // 编码标签和能力状态
	for _, c := range caps {
		enc.SP().Atom(string(c)) // 编码每个能力
	}
	enc.Special(']').SP().Text(text) // 编码结束标记和文本
	return enc.CRLF()                // 写入结束符
}

// writeContReq 写入继续请求。
func writeContReq(enc *imapwire.Encoder, text string) error {
	return enc.Atom("+").SP().Text(text).CRLF() // 编码继续请求
}

// newClientBugError 创建客户端错误。
func newClientBugError(text string) error {
	return &imap.Error{
		Type: imap.StatusResponseTypeBad,
		Code: imap.ResponseCodeClientBug,
		Text: text, // 设置错误信息
	}
}

// UpdateWriter 用于写入状态更新。
type UpdateWriter struct {
	conn         *Conn // 连接
	allowExpunge bool  // 是否允许EXPUNGE
}

// WriteExpunge 写入EXPUNGE响应。
func (w *UpdateWriter) WriteExpunge(seqNum uint32) error {
	if !w.allowExpunge {
		return fmt.Errorf("imapserver：在此上下文中不允许进行 EXPUNGE 更新") // 不允许EXPUNGE
	}
	return w.conn.writeExpunge(seqNum) // 写入EXPUNGE响应
}

// WriteNumMessages 写入EXISTS响应。
func (w *UpdateWriter) WriteNumMessages(n uint32) error {
	return w.conn.writeExists(n) // 写入EXISTS响应
}

// WriteMailboxFlags 写入FLAGS响应。
func (w *UpdateWriter) WriteMailboxFlags(flags []imap.Flag) error {
	return w.conn.writeFlags(flags) // 写入FLAGS响应
}

// WriteMessageFlags 写入FETCH响应带FLAGS。
func (w *UpdateWriter) WriteMessageFlags(seqNum uint32, uid imap.UID, flags []imap.Flag) error {
	fetchWriter := &FetchWriter{conn: w.conn}       // 创建FETCH写入器
	respWriter := fetchWriter.CreateMessage(seqNum) // 创建消息写入器
	if uid != 0 {
		respWriter.WriteUID(uid) // 写入UID
	}
	respWriter.WriteFlags(flags) // 写入FLAGS
	return respWriter.Close()    // 关闭写入器
}
