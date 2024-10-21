package imapserver

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
	"github.com/emersion/go-imap/v2/internal/utf7"
)

// handleList 处理 LIST 命令。
// 参数:
//
//	dec - 解码器，用于读取命令参数。
//
// 返回值:
//
//	处理过程中的错误，如果没有错误返回 nil。
func (c *Conn) handleList(dec *imapwire.Decoder) error {
	ref, pattern, options, returnRecent, err := readListCmd(dec)
	if err != nil {
		return err
	}

	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}

	w := &ListWriter{
		conn:         c,
		options:      options,
		returnRecent: returnRecent,
	}
	return c.session.List(w, ref, pattern, options)
}

// handleLSub 处理 LSUB 命令。
// 参数:
//
//	dec - 解码器，用于读取命令参数。
//
// 返回值:
//
//	处理过程中的错误，如果没有错误返回 nil。
func (c *Conn) handleLSub(dec *imapwire.Decoder) error {
	var ref string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&ref) || !dec.ExpectSP() {
		return dec.Err()
	}
	pattern, err := readListMailbox(dec)
	if err != nil {
		return err
	}
	if !dec.ExpectCRLF() {
		return dec.Err()
	}

	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}

	options := &imap.ListOptions{SelectSubscribed: true}
	w := &ListWriter{
		conn: c,
		lsub: true,
	}
	return c.session.List(w, ref, []string{pattern}, options)
}

// writeList 写入 LIST 响应。
// 参数:
//
//	data - 包含邮箱和属性数据的结构体。
//
// 返回值:
//
//	处理过程中的错误，如果没有错误返回 nil。
func (c *Conn) writeList(data *imap.ListData) error {
	enc := newResponseEncoder(c)
	defer enc.end()

	enc.Atom("*").SP().Atom("LIST").SP()
	enc.List(len(data.Attrs), func(i int) {
		enc.MailboxAttr(data.Attrs[i])
	})
	enc.SP()
	if data.Delim == 0 {
		enc.NIL()
	} else {
		enc.Quoted(string(data.Delim))
	}
	enc.SP().Mailbox(data.Mailbox)

	var ext []string
	if data.ChildInfo != nil {
		ext = append(ext, "子邮箱信息") // CHILDINFO
	}
	if data.OldName != "" {
		ext = append(ext, "旧名称") // OLDNAME
	}

	// TODO: 如果客户端未请求，则省略扩展数据
	if len(ext) > 0 {
		enc.SP().List(len(ext), func(i int) {
			name := ext[i]
			enc.Atom(name).SP()
			switch name {
			case "子邮箱信息": // "CHILDINFO"
				enc.Special('(')
				if data.ChildInfo.Subscribed {
					enc.Quoted("已订阅") // "SUBSCRIBED"
				}
				enc.Special(')')
			case "旧名称": // "OLDNAME"
				enc.Special('(').Mailbox(data.OldName).Special(')')
			default:
				panic(fmt.Errorf("imapserver: 未知的 LIST 扩展项 %v", name)) // "unknown LIST extended-item"
			}
		})
	}

	return enc.CRLF()
}

// writeLSub 写入 LSUB 响应。
// 参数:
//
//	data - 包含邮箱和属性数据的结构体。
//
// 返回值:
//
//	处理过程中的错误，如果没有错误返回 nil。
func (c *Conn) writeLSub(data *imap.ListData) error {
	enc := newResponseEncoder(c)
	defer enc.end()

	enc.Atom("*").SP().Atom("LSUB").SP()
	enc.List(len(data.Attrs), func(i int) {
		enc.MailboxAttr(data.Attrs[i])
	})
	enc.SP()
	if data.Delim == 0 {
		enc.NIL()
	} else {
		enc.Quoted(string(data.Delim))
	}
	enc.SP().Mailbox(data.Mailbox)
	return enc.CRLF()
}

// readListCmd 读取 LIST 命令的参数。
// 返回值:
//
//	ref - 邮箱引用。
//	patterns - 匹配模式。
//	options - 列表选项。
//	returnRecent - 是否返回最近的邮件。
//	err - 处理过程中的错误。
func readListCmd(dec *imapwire.Decoder) (ref string, patterns []string, options *imap.ListOptions, returnRecent bool, err error) {
	options = &imap.ListOptions{}

	if !dec.ExpectSP() {
		return "", nil, nil, false, dec.Err()
	}

	hasSelectOpts, err := dec.List(func() error {
		var selectOpt string
		if !dec.ExpectAString(&selectOpt) {
			return dec.Err()
		}
		switch strings.ToUpper(selectOpt) {
		case "SUBSCRIBED":
			options.SelectSubscribed = true
		case "REMOTE":
			options.SelectRemote = true
		case "RECURSIVEMATCH":
			options.SelectRecursiveMatch = true
		default:
			return newClientBugError("未知的 LIST 选择选项") // "Unknown LIST select option"
		}
		return nil
	})
	if err != nil {
		return "", nil, nil, false, fmt.Errorf("在 list-select-opts 中: %w", err) // "in list-select-opts"
	}
	if hasSelectOpts && !dec.ExpectSP() {
		return "", nil, nil, false, dec.Err()
	}

	if !dec.ExpectMailbox(&ref) || !dec.ExpectSP() {
		return "", nil, nil, false, dec.Err()
	}

	hasPatterns, err := dec.List(func() error {
		pattern, err := readListMailbox(dec)
		if err == nil && pattern != "" {
			patterns = append(patterns, pattern)
		}
		return err
	})
	if err != nil {
		return "", nil, nil, false, err
	} else if hasPatterns && len(patterns) == 0 {
		return "", nil, nil, false, newClientBugError("LIST-EXTENDED 需要一个非空的括号模式列表") // "LIST-EXTENDED requires a non-empty parenthesized pattern list"
	} else if !hasPatterns {
		pattern, err := readListMailbox(dec)
		if err != nil {
			return "", nil, nil, false, err
		}
		if pattern != "" {
			patterns = append(patterns, pattern)
		}
	}

	if dec.SP() { // list-return-opts
		var atom string
		if !dec.ExpectAtom(&atom) || !dec.Expect(strings.EqualFold(atom, "RETURN"), "RETURN") || !dec.ExpectSP() {
			return "", nil, nil, false, dec.Err()
		}

		err := dec.ExpectList(func() error {
			return readReturnOption(dec, options, &returnRecent)
		})
		if err != nil {
			return "", nil, nil, false, fmt.Errorf("在 list-return-opts 中: %w", err) // "in list-return-opts"
		}
	}

	if !dec.ExpectCRLF() {
		return "", nil, nil, false, dec.Err()
	}

	if options.SelectRecursiveMatch && !options.SelectSubscribed {
		return "", nil, nil, false, newClientBugError("LIST RECURSIVEMATCH 选择选项需要 SUBSCRIBED") // "The LIST RECURSIVEMATCH select option requires SUBSCRIBED"
	}

	return ref, patterns, options, returnRecent, nil
}

// readListMailbox 读取 LIST 命令中的邮箱名称。
// 返回值:
//
//	邮箱名称和处理过程中的错误。
func readListMailbox(dec *imapwire.Decoder) (string, error) {
	var mailbox string
	if !dec.String(&mailbox) {
		if !dec.Expect(dec.Func(&mailbox, isListChar), "list-char") {
			return "", dec.Err()
		}
	}
	return utf7.Decode(mailbox)
}

// isListChar 判断字符是否为列表字符。
func isListChar(ch byte) bool {
	switch ch {
	case '%', '*': // 列表通配符
		return true
	case ']': // 响应特殊字符
		return true
	default:
		return imapwire.IsAtomChar(ch)
	}
}

// readReturnOption 读取返回选项。
// 参数:
//
//	dec - 解码器，用于读取命令参数。
//	options - 列表选项。
//	recent - 是否返回最近的邮件。
//
// 返回值:
//
//	处理过程中的错误。
func readReturnOption(dec *imapwire.Decoder, options *imap.ListOptions, recent *bool) error {
	var name string
	if !dec.ExpectAtom(&name) {
		return dec.Err()
	}

	switch strings.ToUpper(name) {
	case "SUBSCRIBED":
		options.ReturnSubscribed = true
	case "CHILDREN":
		options.ReturnChildren = true
	case "STATUS":
		if !dec.ExpectSP() {
			return dec.Err()
		}
		options.ReturnStatus = new(imap.StatusOptions)
		return dec.ExpectList(func() error {
			isRecent, err := readStatusItem(dec, options.ReturnStatus)
			if err != nil {
				return err
			}
			if isRecent {
				*recent = true
			}
			return nil
		})
	default:
		return newClientBugError("未知的 LIST 返回选项") // "Unknown LIST return option"
	}

	return nil
}

// ListWriter 写入 LIST 响应。
type ListWriter struct {
	conn         *Conn             // 连接对象
	options      *imap.ListOptions // 列表选项
	returnRecent bool              // 是否返回最近的邮件
	lsub         bool              // 是否为 LSUB 命令
}

// WriteList 写入单个邮箱的 LIST 响应。
// 参数:
//
//	data - 包含邮箱和属性数据的结构体。
//
// 返回值:
//
//	处理过程中的错误，如果没有错误返回 nil。
func (w *ListWriter) WriteList(data *imap.ListData) error {
	if w.lsub {
		return w.conn.writeLSub(data) // 如果是 LSUB，调用写入 LSUB 的方法
	}

	if err := w.conn.writeList(data); err != nil {
		return err // 写入 LIST 响应时的错误
	}
	if w.options.ReturnStatus != nil && data.Status != nil {
		if err := w.conn.writeStatus(data.Status, w.options.ReturnStatus, w.returnRecent); err != nil {
			return err // 写入状态时的错误
		}
	}
	return nil
}

// MatchList 检查引用和模式是否匹配一个邮箱。
// 参数:
//
//	name - 邮箱名称。
//	delim - 分隔符。
//	reference - 引用。
//	pattern - 匹配模式。
//
// 返回值:
//
//	如果匹配返回 true，否则返回 false。
func MatchList(name string, delim rune, reference, pattern string) bool {
	var delimStr string
	if delim != 0 {
		delimStr = string(delim)
	}

	if delimStr != "" && strings.HasPrefix(pattern, delimStr) {
		reference = ""
		pattern = strings.TrimPrefix(pattern, delimStr)
	}
	if reference != "" {
		if delimStr != "" && !strings.HasSuffix(reference, delimStr) {
			reference += delimStr
		}
		if !strings.HasPrefix(name, reference) {
			return false
		}
		name = strings.TrimPrefix(name, reference)
	}

	return matchList(name, delimStr, pattern)
}

// matchList 检查名称是否与模式匹配。
// 参数:
//
//	name - 邮箱名称。
//	delim - 分隔符。
//	pattern - 匹配模式。
//
// 返回值:
//
//	如果匹配返回 true，否则返回 false。
func matchList(name, delim, pattern string) bool {
	// TODO: 优化

	i := strings.IndexAny(pattern, "*%")
	if i == -1 {
		// 没有更多的通配符
		return name == pattern
	}

	// 获取通配符前后的部分
	chunk, wildcard, rest := pattern[0:i], pattern[i], pattern[i+1:]

	// 检查名称是否以 chunk 开头
	if len(chunk) > 0 && !strings.HasPrefix(name, chunk) {
		return false
	}
	name = strings.TrimPrefix(name, chunk)

	// 展开通配符
	var j int
	for j = 0; j < len(name); j++ {
		if wildcard == '%' && string(name[j]) == delim {
			break // 如果通配符是 %，则在分隔符处停止
		}
		// 尝试从这里匹配其余部分
		if matchList(name[j:], delim, rest) {
			return true
		}
	}

	return matchList(name[j:], delim, rest)
}
