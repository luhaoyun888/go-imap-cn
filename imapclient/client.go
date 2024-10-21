// Package imapclient 实现了一个 IMAP 客户端。
//
// # 字符集解码
//
// 默认情况下，仅执行基本的字符集解码。对于非 UTF-8 的邮件主题和电子邮件地址名称解码，用户可以设置
// Options.WordDecoder。例如，要使用 go-message 的字符集集合：
//
//	import (
//		"mime"
//
//		"github.com/emersion/go-message/charset"
//	)
//
//	options := &imapclient.Options{
//		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
//	}
//	client, err := imapclient.DialTLS("imap.example.org:993", options)

package imapclient

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

const (
	idleReadTimeout    = time.Duration(0) // 空闲读取超时
	respReadTimeout    = 30 * time.Second // 响应读取超时
	literalReadTimeout = 5 * time.Minute  // 文本读取超时

	cmdWriteTimeout     = 30 * time.Second // 命令写入超时
	literalWriteTimeout = 5 * time.Minute  // 文本写入超时
)

var dialer = &net.Dialer{
	Timeout: 30 * time.Second, // 连接超时
}

// SelectedMailbox 包含当前选择的邮箱的元数据。
type SelectedMailbox struct {
	Name           string      // 邮箱名称
	NumMessages    uint32      // 邮件数量
	Flags          []imap.Flag // 邮箱标志
	PermanentFlags []imap.Flag // 永久标志
}

// copy 拷贝
func (mbox *SelectedMailbox) copy() *SelectedMailbox {
	copy := *mbox
	return &copy
}

// Options 包含客户端的选项。
type Options struct {
	// 用于 DialTLS 和 DialStartTLS 的 TLS 配置。如果为 nil，则使用默认配置。
	TLSConfig *tls.Config
	// 原始的输入和输出数据将被写入此写入器（如果有）。注意，这可能包含在身份验证期间使用的敏感信息，例如凭证。
	DebugWriter io.Writer
	// 单边数据处理程序。
	UnilateralDataHandler *UnilateralDataHandler
	// RFC 2047 字符串的解码器。
	WordDecoder *mime.WordDecoder
}

// wrapReadWriter 将读写器包装，如果设置了 DebugWriter，则返回包装后的读写器。
// 否则，返回原始的读写器。
func (options *Options) wrapReadWriter(rw io.ReadWriter) io.ReadWriter {
	// 如果未设置 DebugWriter，则直接返回原始的读写器。
	if options.DebugWriter == nil {
		return rw
	}
	// 返回同时写入 rw 和 DebugWriter 的包装读写器。
	return struct {
		io.Reader
		io.Writer
	}{
		Reader: io.TeeReader(rw, options.DebugWriter),   // 读取时同时写入 DebugWriter
		Writer: io.MultiWriter(rw, options.DebugWriter), // 写入时同时写入 DebugWriter
	}
}

// decodeText 解码 MIME 编码的字符串，返回解码后的字符串。
// 如果没有设置 WordDecoder，则使用默认的 MIME 解码器。
func (options *Options) decodeText(s string) (string, error) {
	wordDecoder := options.WordDecoder
	// 如果没有自定义的 WordDecoder，使用默认的解码器
	if wordDecoder == nil {
		wordDecoder = &mime.WordDecoder{}
	}
	// 使用解码器解码头部
	out, err := wordDecoder.DecodeHeader(s)
	if err != nil {
		return s, err // 解码失败则返回原始字符串和错误
	}
	return out, nil // 返回解码后的结果
}

// unilateralDataHandler 获取单方面数据处理器。
// 如果没有设置自定义的 UnilateralDataHandler，返回一个默认的处理器。
func (options *Options) unilateralDataHandler() *UnilateralDataHandler {
	// 如果未设置 UnilateralDataHandler，返回默认的空处理器
	if options.UnilateralDataHandler == nil {
		return &UnilateralDataHandler{}
	}
	// 返回自定义的 UnilateralDataHandler
	return options.UnilateralDataHandler
}

// tlsConfig 返回 TLS 配置。
// 如果 Options 结构体设置了 TLSConfig，则返回其副本，否则返回默认的 TLS 配置。
func (options *Options) tlsConfig() *tls.Config {
	// 如果 Options 不为空且 TLSConfig 被设置，克隆返回它
	if options != nil && options.TLSConfig != nil {
		return options.TLSConfig.Clone()
	} else {
		// 否则返回一个新的默认 TLS 配置
		return new(tls.Config)
	}
}

// Client 是一个 IMAP 客户端。
//
// IMAP 命令作为方法暴露。这些方法将在命令发送到服务器后阻塞，但不会阻塞直到服务器发送响应。
// 它们返回一个命令结构，可用于等待服务器响应。这可以用于并发执行多个命令，
// 但必须小心以避免歧义。请参阅 RFC 9051 第 5.5 节。
//
// 客户端可以安全地在多个 goroutine 中使用，
// 但这并不保证任何命令的顺序，并且受到命令流水线的相同限制（请参见上文）。
// 此外，一些命令（例如 StartTLS、Authenticate、Idle）在执行期间会阻塞客户端。
type Client struct {
	conn     net.Conn
	options  Options
	br       *bufio.Reader
	bw       *bufio.Writer
	dec      *imapwire.Decoder
	encMutex sync.Mutex

	greetingCh   chan struct{} // 问候通道
	greetingRecv bool          // 是否已接收问候
	greetingErr  error         // 问候错误

	decCh  chan struct{} // 解码通道
	decErr error         // 解码错误

	mutex        sync.Mutex // 互斥锁
	state        imap.ConnState
	caps         imap.CapSet           // 服务器能力集
	enabled      imap.CapSet           // 启用的能力集
	pendingCapCh chan struct{}         // 待处理能力通道
	mailbox      *SelectedMailbox      // 选定的邮箱
	cmdTag       uint64                // 命令标签
	pendingCmds  []command             // 待处理命令
	contReqs     []continuationRequest // 续请求
	closed       bool                  // 是否已关闭
}

// New 创建一个新的 IMAP 客户端。
//
// 此函数不执行 I/O。
//
// nil 选项指针等效于零选项值。
func New(conn net.Conn, options *Options) *Client {
	if options == nil {
		options = &Options{}
	}

	rw := options.wrapReadWriter(conn) // 包装读取器和写入器
	br := bufio.NewReader(rw)          // 创建 bufio 读取器
	bw := bufio.NewWriter(rw)          // 创建 bufio 写入器

	client := &Client{
		conn:       conn,
		options:    *options,
		br:         br,
		bw:         bw,
		dec:        imapwire.NewDecoder(br, imapwire.ConnSideClient),
		greetingCh: make(chan struct{}), // 初始化问候通道
		decCh:      make(chan struct{}), // 初始化解码通道
		state:      imap.ConnStateNone,  // 初始化连接状态
		enabled:    make(imap.CapSet),   // 初始化启用的能力集
	}
	go client.read() // 启动读取 goroutine
	return client
}

// NewStartTLS 创建一个新的 IMAP 客户端，使用 STARTTLS。
//
// nil 选项指针等效于零选项值。
func NewStartTLS(conn net.Conn, options *Options) (*Client, error) {
	if options == nil {
		options = &Options{}
	}

	client := New(conn, options) // 创建新的客户端
	if err := client.startTLS(options.TLSConfig); err != nil {
		conn.Close()
		return nil, err // 启用 STARTTLS 失败
	}

	// 根据第 7.1.4 节，在使用 STARTTLS 时拒绝 PREAUTH
	if client.State() != imap.ConnStateNotAuthenticated {
		client.Close()
		return nil, fmt.Errorf("imapclient: 服务器在未加密连接上发送了 PREAUTH")
	}

	return client, nil
}

// DialInsecure 连接到不加密的 IMAP 服务器。
func DialInsecure(address string, options *Options) (*Client, error) {
	conn, err := net.Dial("tcp", address) // 建立 TCP 连接
	if err != nil {
		return nil, err
	}
	return New(conn, options), nil // 创建并返回客户端
}

// DialTLS 连接到使用隐式 TLS 的 IMAP 服务器。
func DialTLS(address string, options *Options) (*Client, error) {
	tlsConfig := options.tlsConfig() // 获取 TLS 配置
	if tlsConfig.NextProtos == nil {
		tlsConfig.NextProtos = []string{"imap"} // 设置下一个协议
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig) // 使用 TLS 建立连接
	if err != nil {
		return nil, err
	}
	return New(conn, options), nil // 创建并返回客户端
}

// DialStartTLS 连接到使用 STARTTLS 的 IMAP 服务器。
func DialStartTLS(address string, options *Options) (*Client, error) {
	if options == nil {
		options = &Options{}
	}

	host, _, err := net.SplitHostPort(address) // 拆分主机和端口
	if err != nil {
		return nil, err
	}

	conn, err := dialer.Dial("tcp", address) // 建立 TCP 连接
	if err != nil {
		return nil, err
	}

	tlsConfig := options.tlsConfig() // 获取 TLS 配置
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = host // 设置服务器名称
	}
	newOptions := *options
	newOptions.TLSConfig = tlsConfig      // 更新选项中的 TLS 配置
	return NewStartTLS(conn, &newOptions) // 创建并返回 STARTTLS 客户端
}

func (c *Client) setReadTimeout(dur time.Duration) {
	if dur > 0 {
		c.conn.SetReadDeadline(time.Now().Add(dur)) // 设置读取超时
	} else {
		c.conn.SetReadDeadline(time.Time{}) // 取消读取超时
	}
}

func (c *Client) setWriteTimeout(dur time.Duration) {
	if dur > 0 {
		c.conn.SetWriteDeadline(time.Now().Add(dur)) // 设置写入超时
	} else {
		c.conn.SetWriteDeadline(time.Time{}) // 取消写入超时
	}
}

// State 返回客户端当前的连接状态。
func (c *Client) State() imap.ConnState {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.state // 返回当前状态
}

func (c *Client) setState(state imap.ConnState) {
	c.mutex.Lock()
	c.state = state // 设置连接状态
	if c.state != imap.ConnStateSelected {
		c.mailbox = nil // 如果不是选定状态，清空邮箱
	}
	c.mutex.Unlock()
}

// Caps 返回服务器通告的能力。
//
// 当服务器尚未发送能力列表时，此方法将请求它并阻塞，直到接收到。如果无法获取能力，则返回 nil。
func (c *Client) Caps() imap.CapSet {
	if err := c.WaitGreeting(); err != nil {
		return nil // 如果问候失败，返回 nil
	}

	c.mutex.Lock()
	caps := c.caps
	capCh := c.pendingCapCh // 获取待处理能力通道
	c.mutex.Unlock()

	if caps != nil {
		return caps // 如果能力已设置，直接返回
	}

	if capCh == nil {
		capCmd := c.Capability()     // 请求能力
		capCh := make(chan struct{}) // 创建能力通道
		go func() {
			capCmd.Wait() // 等待能力命令完成
			close(capCh)  // 关闭通道
		}()
		c.mutex.Lock()
		c.pendingCapCh = capCh // 设置待处理能力通道
		c.mutex.Unlock()
	}

	timer := time.NewTimer(respReadTimeout) // 创建超时定时器
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil // 超时返回 nil
	case <-capCh:
		// ok
	}

	// TODO: 如果在我们收到回复之前重置能力，这是不安全的
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.caps // 返回能力
}

func (c *Client) setCaps(caps imap.CapSet) {
	// 如果能力被重置，则从服务器请求更新的能力
	var capCh chan struct{}
	if caps == nil {
		capCh = make(chan struct{})

		// 我们需要在单独的 goroutine 中发送 CAPABILITY 命令：
		// setCaps 可能在 Client.encMutex 锁定时被调用
		go func() {
			c.Capability().Wait() // 等待能力命令完成
			close(capCh)          // 关闭通道
		}()
	}

	c.mutex.Lock()
	c.caps = caps          // 设置能力
	c.pendingCapCh = capCh // 设置待处理能力通道
	c.mutex.Unlock()
}

// Mailbox 返回当前选定邮箱的状态。
//
// 如果没有当前选定的邮箱，则返回 nil。
//
// 返回的结构体不得被修改。
func (c *Client) Mailbox() *SelectedMailbox {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.mailbox // 返回选定的邮箱
}

// Close 立即关闭连接。
func (c *Client) Close() error {
	c.mutex.Lock()
	alreadyClosed := c.closed // 检查是否已关闭
	c.closed = true
	c.mutex.Unlock()

	// 在这里忽略 net.ErrClosed，因为我们在 c.read 中也调用了 conn.Close
	if err := c.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		return err // 返回关闭错误
	}

	<-c.decCh // 等待解码通道关闭
	if err := c.decErr; err != nil {
		return err // 返回解码错误
	}

	if alreadyClosed {
		return net.ErrClosed // 如果已经关闭，返回已关闭错误
	}
	return nil
}

// beginCommand 开始向服务器发送命令。
//
// 命令名称和一个空格被写入。
//
// 调用者必须调用 commandEncoder.end。
func (c *Client) beginCommand(name string, cmd command) *commandEncoder {
	c.encMutex.Lock() // commandEncoder.end 解锁

	c.mutex.Lock()

	c.cmdTag++                          // 增加命令标签
	tag := fmt.Sprintf("T%v", c.cmdTag) // 格式化标签

	baseCmd := cmd.base()
	*baseCmd = commandBase{
		tag:  tag,
		done: make(chan error, 1), // 创建命令完成通道
	}

	c.pendingCmds = append(c.pendingCmds, cmd) // 将命令添加到待处理命令中
	quotedUTF8 := c.caps.Has(imap.CapIMAP4rev2) || c.enabled.Has(imap.CapUTF8Accept)
	literalMinus := c.caps.Has(imap.CapLiteralMinus)
	literalPlus := c.caps.Has(imap.CapLiteralPlus)

	c.mutex.Unlock()

	c.setWriteTimeout(cmdWriteTimeout) // 设置写入超时

	wireEnc := imapwire.NewEncoder(c.bw, imapwire.ConnSideClient) // 创建编码器
	wireEnc.QuotedUTF8 = quotedUTF8
	wireEnc.LiteralMinus = literalMinus
	wireEnc.LiteralPlus = literalPlus
	wireEnc.NewContinuationRequest = func() *imapwire.ContinuationRequest {
		return c.registerContReq(cmd) // 注册续请求
	}

	enc := &commandEncoder{
		Encoder: wireEnc,
		client:  c,
		cmd:     baseCmd,
	}
	enc.Atom(tag).SP().Atom(name) // 编码命令
	return enc
}

// deletePendingCmdByTag 根据命令的标签删除队列中的待处理命令。
// 参数：
// - tag: 字符串类型，表示要删除的命令标签。
// 返回值：
// - 返回被删除的命令，如果未找到则返回 nil。
func (c *Client) deletePendingCmdByTag(tag string) command {
	// 加锁，防止并发操作
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 遍历待处理命令队列，找到匹配的标签
	for i, cmd := range c.pendingCmds {
		if cmd.base().tag == tag {
			// 删除找到的命令
			c.pendingCmds = append(c.pendingCmds[:i], c.pendingCmds[i+1:]...)
			return cmd
		}
	}
	// 如果未找到命令，返回 nil
	return nil
}

// findPendingCmdFunc 根据传入的函数条件查找待处理命令。
// 参数：
// - f: 函数类型，接收一个命令并返回布尔值，决定是否匹配。
// 返回值：
// - 返回匹配的命令，如果未找到则返回 nil。
func (c *Client) findPendingCmdFunc(f func(cmd command) bool) command {
	// 加锁，防止并发操作
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 遍历待处理命令，找到符合条件的命令
	for _, cmd := range c.pendingCmds {
		if f(cmd) {
			return cmd
		}
	}
	// 如果未找到命令，返回 nil
	return nil
}

// findPendingCmdByType 根据命令类型查找待处理命令。
// 泛型函数，返回类型 T 必须实现 command 接口。
// 返回值：
// - 返回指定类型的命令，如果未找到则返回空的 T 类型值。
func findPendingCmdByType[T command](c *Client) T {
	// 加锁，防止并发操作
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 遍历待处理命令队列，找到指定类型的命令
	for _, cmd := range c.pendingCmds {
		if cmd, ok := cmd.(T); ok {
			return cmd
		}
	}

	// 如果未找到匹配的命令，返回空的类型 T 值
	var cmd T
	return cmd
}

// completeCommand 标记命令为完成，并根据错误情况更新连接状态。
// 参数：
// - cmd: 待完成的命令。
// - err: 错误信息，命令成功时为 nil。
func (c *Client) completeCommand(cmd command, err error) {
	// 获取命令的完成通道并发送错误信息
	done := cmd.base().done
	done <- err
	close(done)

	// 确保命令不会因为后续请求被阻塞
	c.mutex.Lock()
	var filtered []continuationRequest
	for _, contReq := range c.contReqs {
		if contReq.cmd != cmd.base() {
			filtered = append(filtered, contReq)
		} else {
			contReq.Cancel(err)
		}
	}
	c.contReqs = filtered
	c.mutex.Unlock()

	// 根据命令类型更新客户端状态
	switch cmd := cmd.(type) {
	case *authenticateCommand, *loginCommand:
		if err == nil {
			c.setState(imap.ConnStateAuthenticated) // 设置为已认证状态
		}
	case *unauthenticateCommand:
		if err == nil {
			c.mutex.Lock()
			c.state = imap.ConnStateNotAuthenticated // 设置为未认证状态
			c.mailbox = nil
			c.enabled = make(imap.CapSet) // 重置已启用的能力集
			c.mutex.Unlock()
		}
	case *SelectCommand:
		if err == nil {
			c.mutex.Lock()
			c.state = imap.ConnStateSelected // 设置为已选择状态
			c.mailbox = &SelectedMailbox{
				Name:           cmd.mailbox,             // 邮箱名称
				NumMessages:    cmd.data.NumMessages,    // 邮件数量
				Flags:          cmd.data.Flags,          // 标志
				PermanentFlags: cmd.data.PermanentFlags, // 永久标志
			}
			c.mutex.Unlock()
		}
	case *unselectCommand:
		if err == nil {
			c.setState(imap.ConnStateAuthenticated) // 重置为已认证状态
		}
	case *logoutCommand:
		if err == nil {
			c.setState(imap.ConnStateLogout) // 设置为已注销状态
		}
	case *ListCommand:
		if cmd.pendingData != nil {
			cmd.mailboxes <- cmd.pendingData // 发送待处理的邮箱数据
		}
		close(cmd.mailboxes) // 关闭邮箱通道
	case *FetchCommand:
		close(cmd.msgs) // 关闭消息通道
	case *ExpungeCommand:
		close(cmd.seqNums) // 关闭序列号通道
	}
}

// registerContReq 注册一个后续请求，用于处理命令的进一步交互。
// 参数：
// - cmd: 要注册的命令。
// 返回值：
// - 返回创建的继续请求。
func (c *Client) registerContReq(cmd command) *imapwire.ContinuationRequest {
	// 创建新的继续请求
	contReq := imapwire.NewContinuationRequest()

	// 加锁并添加到继续请求队列中
	c.mutex.Lock()
	c.contReqs = append(c.contReqs, continuationRequest{
		ContinuationRequest: contReq,
		cmd:                 cmd.base(),
	})
	c.mutex.Unlock()

	return contReq
}

// closeWithError 关闭客户端连接并为所有待处理命令返回错误。
// 参数：
// - err: 关闭时发生的错误。
func (c *Client) closeWithError(err error) {
	// 关闭连接
	c.conn.Close()

	// 加锁并清空待处理命令队列
	c.mutex.Lock()
	c.state = imap.ConnStateLogout // 设置为已注销状态
	pendingCmds := c.pendingCmds
	c.pendingCmds = nil
	c.mutex.Unlock()

	// 为每个待处理的命令标记为完成并返回错误
	for _, cmd := range pendingCmds {
		c.completeCommand(cmd, err)
	}
}

// read 方法持续读取从服务器返回的数据。
//
// 所有数据都在读取的 goroutine 中进行解码，然后通过通道分发到待处理的命令中。
func (c *Client) read() {
	// 关闭解码通道
	defer close(c.decCh)
	defer func() {
		if v := recover(); v != nil {
			c.decErr = fmt.Errorf("imapclient: 读取响应时发生 panic: %v\n%s", v, debug.Stack()) // 发生 panic 时，记录错误信息
		}

		cmdErr := c.decErr
		if cmdErr == nil {
			cmdErr = io.ErrUnexpectedEOF // 如果未定义错误，默认为意外的 EOF 错误
		}
		c.closeWithError(cmdErr) // 关闭连接并传递错误信息
	}()

	// 设置读取超时时间，等待服务器问候消息
	c.setReadTimeout(respReadTimeout)
	for {
		// 忽略 net.ErrClosed 错误，因为在 c.Close 中也调用了 conn.Close
		if c.dec.EOF() || errors.Is(c.dec.Err(), net.ErrClosed) || errors.Is(c.dec.Err(), io.ErrClosedPipe) {
			break
		}
		if err := c.readResponse(); err != nil {
			c.decErr = err
			break
		}
		if c.greetingErr != nil {
			break
		}
	}
}

// readResponse 读取并处理服务器的响应。
// 返回值：
// - 返回读取的错误信息，若无错误则返回 nil。
func (c *Client) readResponse() error {
	// 设置读取超时时间
	c.setReadTimeout(respReadTimeout)
	defer c.setReadTimeout(idleReadTimeout) // 完成读取后重置为空闲状态的超时

	// 检查是否为继续请求
	if c.dec.Special('+') {
		if err := c.readContinueReq(); err != nil {
			return fmt.Errorf("在继续请求中: %v", err)
		}
		return nil
	}

	// 解析响应中的标签和类型
	var tag, typ string
	if !c.dec.Expect(c.dec.Special('*') || c.dec.Atom(&tag), "'*' 或原子") {
		return fmt.Errorf("响应中: 无法读取标签: %v", c.dec.Err())
	}
	if !c.dec.ExpectSP() {
		return fmt.Errorf("响应中: %v", c.dec.Err())
	}
	if !c.dec.ExpectAtom(&typ) {
		return fmt.Errorf("响应中: 无法读取类型: %v", c.dec.Err())
	}

	// 根据是否有标签处理响应
	var (
		token    string
		err      error
		startTLS *startTLSCommand
	)
	if tag != "" {
		token = "有标签的响应"
		startTLS, err = c.readResponseTagged(tag, typ)
	} else {
		token = "数据响应"
		err = c.readResponseData(typ)
	}
	if err != nil {
		return fmt.Errorf("在 %v 中: %v", token, err)
	}

	// 检查响应结束
	if !c.dec.ExpectCRLF() {
		return fmt.Errorf("响应中: %v", c.dec.Err())
	}

	// 如果是 STARTTLS 命令，则升级为安全连接
	if startTLS != nil {
		c.upgradeStartTLS(startTLS)
	}

	return nil
}

// readContinueReq 读取服务器发送的继续请求。
// 返回值：
// - 返回读取的错误信息，若无错误则返回 nil。
func (c *Client) readContinueReq() error {
	// 读取继续请求的文本
	var text string
	if c.dec.SP() {
		c.dec.Text(&text)
	}
	if !c.dec.ExpectCRLF() {
		return c.dec.Err()
	}

	// 从继续请求队列中获取匹配的请求
	var contReq *imapwire.ContinuationRequest
	c.mutex.Lock()
	if len(c.contReqs) > 0 {
		contReq = c.contReqs[0].ContinuationRequest
		c.contReqs = append(c.contReqs[:0], c.contReqs[1:]...)
	}
	c.mutex.Unlock()

	if contReq == nil {
		return fmt.Errorf("收到未匹配的继续请求")
	}

	// 完成继续请求并返回文本
	contReq.Done(text)
	return nil
}

// readResponseTagged 读取服务器发送的带标签响应，并处理对应的命令。
//
// 参数：
// - tag: 响应的标签，用于匹配待处理的命令。
// - typ: 响应的类型，表示状态，例如 OK、NO、BAD 等。
//
// 返回值：
// - startTLS: 如果响应中包含 STARTTLS 命令，则返回对应的命令。
// - err: 返回处理过程中发生的错误，若无错误则为 nil。
func (c *Client) readResponseTagged(tag, typ string) (startTLS *startTLSCommand, err error) {
	// 根据标签删除并返回待处理的命令
	cmd := c.deletePendingCmdByTag(tag)
	if cmd == nil {
		return nil, fmt.Errorf("收到未知标签 %q 的带标签响应", tag)
	}

	// 已将命令从待处理队列中删除，确保在错误发生时不会阻塞命令。
	defer func() {
		if err != nil {
			c.completeCommand(cmd, err)
		}
	}()

	// 某些服务器即使 RFC 要求文本也不会提供，参考问题 #500 和 #502
	hasSP := c.dec.SP()

	var code string
	if hasSP && c.dec.Special('[') { // 处理 resp-text-code 部分
		if !c.dec.ExpectAtom(&code) {
			return nil, fmt.Errorf("在 resp-text-code 中: %v", c.dec.Err())
		}
		// 处理可能的文本代码
		switch code {
		case "CAPABILITY": // 解析 CAPABILITY 数据
			caps, err := readCapabilities(c.dec)
			if err != nil {
				return nil, fmt.Errorf("在 capability-data 中: %v", err)
			}
			c.setCaps(caps) // 设置客户端的功能集
		case "APPENDUID":
			var (
				uidValidity uint32
				uid         imap.UID
			)
			// 读取 APPENDUID 相关的有效性和 UID
			if !c.dec.ExpectSP() || !c.dec.ExpectNumber(&uidValidity) || !c.dec.ExpectSP() || !c.dec.ExpectUID(&uid) {
				return nil, fmt.Errorf("在 resp-code-apnd 中: %v", c.dec.Err())
			}
			if cmd, ok := cmd.(*AppendCommand); ok {
				cmd.data.UID = uid
				cmd.data.UIDValidity = uidValidity
			}
		case "COPYUID":
			if !c.dec.ExpectSP() {
				return nil, c.dec.Err()
			}
			// 读取 COPYUID 相关的有效性和 UID
			uidValidity, srcUIDs, dstUIDs, err := readRespCodeCopyUID(c.dec)
			if err != nil {
				return nil, fmt.Errorf("在 resp-code-copy 中: %v", err)
			}
			// 处理命令类型为 CopyCommand 或 MoveCommand
			switch cmd := cmd.(type) {
			case *CopyCommand:
				cmd.data.UIDValidity = uidValidity
				cmd.data.SourceUIDs = srcUIDs
				cmd.data.DestUIDs = dstUIDs
			case *MoveCommand:
				// 当 Client.Move 回退到 COPY + STORE + EXPUNGE 时可能会发生这种情况
				cmd.data.UIDValidity = uidValidity
				cmd.data.SourceUIDs = srcUIDs
				cmd.data.DestUIDs = dstUIDs
			}
		default: // 处理其他未定义的文本代码
			if c.dec.SP() {
				c.dec.DiscardUntilByte(']')
			}
		}
		if !c.dec.ExpectSpecial(']') {
			return nil, fmt.Errorf("在 resp-text 中: %v", c.dec.Err())
		}
		hasSP = c.dec.SP()
	}

	// 读取响应的文本部分
	var text string
	if hasSP && !c.dec.ExpectText(&text) {
		return nil, fmt.Errorf("在 resp-text 中: %v", c.dec.Err())
	}

	// 根据响应类型处理不同的状态
	var cmdErr error
	switch typ {
	case "OK":
		// 不需要处理 OK 类型的响应
	case "NO", "BAD":
		cmdErr = &imap.Error{
			Type: imap.StatusResponseType(typ),
			Code: imap.ResponseCode(code),
			Text: text,
		}
	default:
		return nil, fmt.Errorf("在 resp-cond-state 中: 期望 OK、NO 或 BAD 状态，但收到 %v", typ)
	}

	// 完成命令处理并传递可能的错误
	c.completeCommand(cmd, cmdErr)

	// 处理 STARTTLS 命令
	if cmd, ok := cmd.(*startTLSCommand); ok && cmdErr == nil {
		startTLS = cmd
	}

	// 如果没有错误并且代码不是 CAPABILITY，清空某些命令的功能集
	if cmdErr == nil && code != "CAPABILITY" {
		switch cmd.(type) {
		case *startTLSCommand, *loginCommand, *authenticateCommand, *unauthenticateCommand:
			// 这些命令会使功能集无效
			c.setCaps(nil)
		}
	}

	return startTLS, nil
}

// readResponseData 解析服务器的响应数据，根据响应类型处理相应的逻辑。
// 参数：
// - typ: 响应的类型，可能是存在、最近、抓取、删除等。
func (c *Client) readResponseData(typ string) error {
	// 数字 SP ("EXISTS" / "RECENT" / "FETCH" / "EXPUNGE")
	var num uint32
	// 如果类型是数字开头，则解析该数字
	if typ[0] >= '0' && typ[0] <= '9' {
		v, err := strconv.ParseUint(typ, 10, 32)
		if err != nil {
			return err
		}

		num = uint32(v)
		// 检查是否有 SP 和 atom 类型
		if !c.dec.ExpectSP() || !c.dec.ExpectAtom(&typ) {
			return c.dec.Err()
		}
	}

	// 根据不同的响应类型进行处理
	switch typ {
	case "OK", "PREAUTH", "NO", "BAD", "BYE": // 响应状态：OK，预授权，NO，BAD，BYE
		// 某些服务器不会提供文本，即使 RFC 要求，参见 #500 和 #502
		hasSP := c.dec.SP()

		var code string
		if hasSP && c.dec.Special('[') { // resp-text-code 响应文本代码
			if !c.dec.ExpectAtom(&code) {
				return fmt.Errorf("在 resp-text-code 中出错: %v", c.dec.Err())
			}
			// 根据不同的代码处理
			switch code {
			case "CAPABILITY": // 读取功能数据
				caps, err := readCapabilities(c.dec)
				if err != nil {
					return fmt.Errorf("在 capability-data 中出错: %v", err)
				}
				c.setCaps(caps)
			case "PERMANENTFLAGS": // 处理永久标志
				if !c.dec.ExpectSP() {
					return c.dec.Err()
				}
				flags, err := internal.ExpectFlagList(c.dec)
				if err != nil {
					return err
				}

				// 更新邮箱永久标志
				c.mutex.Lock()
				if c.state == imap.ConnStateSelected {
					c.mailbox = c.mailbox.copy()
					c.mailbox.PermanentFlags = flags
				}
				c.mutex.Unlock()

				// 检查是否有等待处理的命令
				if cmd := findPendingCmdByType[*SelectCommand](c); cmd != nil {
					cmd.data.PermanentFlags = flags
				} else if handler := c.options.unilateralDataHandler().Mailbox; handler != nil {
					handler(&UnilateralDataMailbox{PermanentFlags: flags})
				}
			case "UIDNEXT":
				var uidNext imap.UID
				if !c.dec.ExpectSP() || !c.dec.ExpectUID(&uidNext) {
					return c.dec.Err()
				}
				if cmd := findPendingCmdByType[*SelectCommand](c); cmd != nil {
					cmd.data.UIDNext = uidNext
				}
			case "UIDVALIDITY":
				var uidValidity uint32
				if !c.dec.ExpectSP() || !c.dec.ExpectNumber(&uidValidity) {
					return c.dec.Err()
				}
				if cmd := findPendingCmdByType[*SelectCommand](c); cmd != nil {
					cmd.data.UIDValidity = uidValidity
				}
			case "COPYUID":
				if !c.dec.ExpectSP() {
					return c.dec.Err()
				}
				uidValidity, srcUIDs, dstUIDs, err := readRespCodeCopyUID(c.dec)
				if err != nil {
					return fmt.Errorf("在 resp-code-copy 中出错: %v", err)
				}
				if cmd := findPendingCmdByType[*MoveCommand](c); cmd != nil {
					cmd.data.UIDValidity = uidValidity
					cmd.data.SourceUIDs = srcUIDs
					cmd.data.DestUIDs = dstUIDs
				}
			case "HIGHESTMODSEQ":
				var modSeq uint64
				if !c.dec.ExpectSP() || !c.dec.ExpectModSeq(&modSeq) {
					return c.dec.Err()
				}
				if cmd := findPendingCmdByType[*SelectCommand](c); cmd != nil {
					cmd.data.HighestModSeq = modSeq
				}
			case "NOMODSEQ":
				// 忽略
			default: // [SP 1*<任意除了 "]" 的文本字符>]
				if c.dec.SP() {
					c.dec.DiscardUntilByte(']')
				}
			}
			if !c.dec.ExpectSpecial(']') {
				return fmt.Errorf("在 resp-text 中出错: %v", c.dec.Err())
			}
			hasSP = c.dec.SP()
		}

		var text string
		if hasSP && !c.dec.ExpectText(&text) {
			return fmt.Errorf("在 resp-text 中出错: %v", c.dec.Err())
		}

		if code == "CLOSED" {
			c.setState(imap.ConnStateAuthenticated)
		}

		if !c.greetingRecv {
			switch typ {
			case "OK":
				c.setState(imap.ConnStateNotAuthenticated)
			case "PREAUTH":
				c.setState(imap.ConnStateAuthenticated)
			default:
				c.setState(imap.ConnStateLogout)
				c.greetingErr = &imap.Error{
					Type: imap.StatusResponseType(typ),
					Code: imap.ResponseCode(code),
					Text: text,
				}
			}
			c.greetingRecv = true
			if c.greetingErr == nil && code != "CAPABILITY" {
				c.setCaps(nil) // 请求初始功能
			}
			close(c.greetingCh)
		}
	case "ID":
		return c.handleID()
	case "CAPABILITY":
		return c.handleCapability()
	case "ENABLED":
		return c.handleEnabled()
	case "NAMESPACE":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleNamespace()
	case "FLAGS":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleFlags()
	case "EXISTS":
		return c.handleExists(num)
	case "RECENT":
		// 忽略
	case "LIST":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleList()
	case "STATUS":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleStatus()
	case "FETCH":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleFetch(num)
	case "EXPUNGE":
		return c.handleExpunge(num)
	case "SEARCH":
		return c.handleSearch()
	case "ESEARCH":
		return c.handleESearch()
	case "SORT":
		return c.handleSort()
	case "THREAD":
		return c.handleThread()
	case "METADATA":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleMetadata()
	case "QUOTA":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleQuota()
	case "QUOTAROOT":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleQuotaRoot()
	case "MYRIGHTS":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleMyRights()
	case "ACL":
		if !c.dec.ExpectSP() {
			return c.dec.Err()
		}
		return c.handleGetACL()
	default:
		return fmt.Errorf("不支持的响应类型 %q", typ)
	}

	return nil
}

// WaitGreeting 等待服务器的初始问候响应。
func (c *Client) WaitGreeting() error {
	select {
	case <-c.greetingCh: // 等待问候通道关闭
		return c.greetingErr
	case <-c.decCh: // 等待解码通道
		if c.decErr != nil {
			return fmt.Errorf("在问候前出现错误: %v", c.decErr) // 返回解码错误
		}
		return fmt.Errorf("在问候前连接已关闭") // 连接在问候前关闭
	}
}

// Noop 发送 NOOP 命令。
func (c *Client) Noop() *Command {
	cmd := &Command{}
	c.beginCommand("NOOP", cmd).end() // 开始并结束 NOOP 命令
	return cmd
}

// Logout 发送 LOGOUT 命令，通知服务器客户端已完成连接。
func (c *Client) Logout() *Command {
	cmd := &logoutCommand{}
	c.beginCommand("LOGOUT", cmd).end() // 开始并结束 LOGOUT 命令
	return &cmd.Command
}

// Login 发送 LOGIN 命令。
func (c *Client) Login(username, password string) *Command {
	cmd := &loginCommand{}
	enc := c.beginCommand("LOGIN", cmd)             // 开始登录命令
	enc.SP().String(username).SP().String(password) // 添加用户名和密码
	enc.end()                                       // 结束命令
	return &cmd.Command
}

// Delete 发送 DELETE 命令。
func (c *Client) Delete(mailbox string) *Command {
	cmd := &Command{}
	enc := c.beginCommand("DELETE", cmd)
	enc.SP().Mailbox(mailbox) // 添加邮箱名
	enc.end()                 // 结束命令
	return cmd
}

// Rename 发送 RENAME 命令。
func (c *Client) Rename(mailbox, newName string) *Command {
	cmd := &Command{}
	enc := c.beginCommand("RENAME", cmd)
	enc.SP().Mailbox(mailbox).SP().Mailbox(newName) // 添加旧邮箱名和新邮箱名
	enc.end()                                       // 结束命令
	return cmd
}

// Subscribe 发送 SUBSCRIBE 命令。
func (c *Client) Subscribe(mailbox string) *Command {
	cmd := &Command{}
	enc := c.beginCommand("SUBSCRIBE", cmd)
	enc.SP().Mailbox(mailbox) // 添加邮箱名
	enc.end()                 // 结束命令
	return cmd
}

// Unsubscribe 发送 UNSUBSCRIBE 命令。
func (c *Client) Unsubscribe(mailbox string) *Command {
	cmd := &Command{}
	enc := c.beginCommand("UNSUBSCRIBE", cmd)
	enc.SP().Mailbox(mailbox) // 添加邮箱名
	enc.end()                 // 结束命令
	return cmd
}

// uidCmdName 根据 NumKind 类型返回适当的命令名称。
// 参数：
// - name: 原始命令名称。
// - kind: 表示数字类型的 imapwire.NumKind。
func uidCmdName(name string, kind imapwire.NumKind) string {
	switch kind {
	case imapwire.NumKindSeq:
		return name
	case imapwire.NumKindUID:
		return "UID " + name
	default:
		panic("imapclient: 无效的 imapwire.NumKind")
	}
}

// commandEncoder 是用于编码 IMAP 命令的结构体。
// 字段：
// - Encoder: 用于实际编码命令的 Encoder。
// - client: 关联的 IMAP 客户端。
// - cmd: 关联的命令基础结构。
type commandEncoder struct {
	*imapwire.Encoder
	client *Client
	cmd    *commandBase
}

// end 结束一个正在发送的命令。
// 该方法写入 CRLF，刷新编码器并释放锁。
func (ce *commandEncoder) end() {
	if ce.Encoder != nil {
		ce.flush()
	}
	ce.client.setWriteTimeout(0)
	ce.client.encMutex.Unlock()
}

// flush 发送一个正在发送的命令，但保持编码器的锁。
// 该方法写入 CRLF 并刷新编码器。调用者必须调用 commandEncoder.end 来释放锁。
func (ce *commandEncoder) flush() {
	if err := ce.Encoder.CRLF(); err != nil {
		// TODO: 考虑将错误存储在 Client 中，以便在未来调用中返回
		ce.client.closeWithError(err)
	}
	ce.Encoder = nil
}

// Literal 编码一个字面量。
// 参数：
// - size: 字面量的大小。
// 返回：
// - io.WriteCloser: 用于写入字面量的 WriteCloser。
func (ce *commandEncoder) Literal(size int64) io.WriteCloser {
	var contReq *imapwire.ContinuationRequest
	ce.client.mutex.Lock()
	hasCapLiteralMinus := ce.client.caps.Has(imap.CapLiteralMinus)
	ce.client.mutex.Unlock()
	if size > 4096 || !hasCapLiteralMinus {
		contReq = ce.client.registerContReq(ce.cmd)
	}
	ce.client.setWriteTimeout(literalWriteTimeout)
	return literalWriter{
		WriteCloser: ce.Encoder.Literal(size, contReq),
		client:      ce.client,
	}
}

// literalWriter 是用于写入字面量的结构体。
// 字段：
// - WriteCloser: 实际的写入器。
// - client: 关联的 IMAP 客户端。
type literalWriter struct {
	io.WriteCloser
	client *Client
}

// Close 关闭字面量写入器。
// 返回：
// - error: 如果有错误，返回错误。
func (lw literalWriter) Close() error {
	lw.client.setWriteTimeout(cmdWriteTimeout)
	return lw.WriteCloser.Close()
}

// continuationRequest 表示一个挂起的继续请求。
// 字段：
// - ContinuationRequest: 实际的继续请求。
// - cmd: 关联的命令基础结构。
type continuationRequest struct {
	*imapwire.ContinuationRequest
	cmd *commandBase
}

// UnilateralDataMailbox 描述邮箱状态更新。
// 字段：
// - NumMessages: 邮箱中的消息数。如果为空，表示没有改变。
// - Flags: 邮箱的标志数组。
// - PermanentFlags: 邮箱的永久标志数组。
type UnilateralDataMailbox struct {
	NumMessages    *uint32
	Flags          []imap.Flag
	PermanentFlags []imap.Flag
}

// UnilateralDataHandler 处理单方面的数据。
// 字段：
// - Expunge: 处理删除消息的函数，参数是消息的序列号。
// - Mailbox: 处理邮箱状态更新的函数，参数是 UnilateralDataMailbox。
// - Fetch: 处理抓取消息的函数，参数是 FetchMessageData。
// - Metadata: 处理邮箱元数据的函数，要求启用 METADATA 或 SERVER-METADATA。
type UnilateralDataHandler struct {
	Expunge  func(seqNum uint32)
	Mailbox  func(data *UnilateralDataMailbox)
	Fetch    func(msg *FetchMessageData)
	Metadata func(mailbox string, entries []string)
}

// command 是 IMAP 命令的接口。
// base 返回命令的基础结构。
type command interface {
	base() *commandBase
}

// commandBase 是 IMAP 命令的基础结构。
// 字段：
// - tag: 命令的标识。
// - done: 一个信道，表示命令是否完成。
// - err: 命令的错误。
type commandBase struct {
	tag  string
	done chan error
	err  error
}

// base 返回命令的基础结构。
func (cmd *commandBase) base() *commandBase {
	return cmd
}

// wait 等待命令完成。
// 返回：
// - error: 如果有错误，返回错误。
func (cmd *commandBase) wait() error {
	if cmd.err == nil {
		cmd.err = <-cmd.done
	}
	return cmd.err
}

// Command 是一个基础的 IMAP 命令。
// 它继承了 commandBase。
type Command struct {
	commandBase
}

// Wait 阻塞直到命令完成。
// 返回：
// - error: 如果有错误，返回错误。
func (cmd *Command) Wait() error {
	return cmd.wait()
}

// loginCommand 是一个登录命令，继承自 Command。
type loginCommand struct {
	Command
}

// logoutCommand 是一个注销命令，继承自 Command。
type logoutCommand struct {
	Command
}
