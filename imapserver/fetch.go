package imapserver

import (
	"fmt"
	"io"
	"mime"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

const envelopeDateLayout = "Mon, 02 Jan 2006 15:04:05 -0700"

// fetchWriterOptions 是用于配置 FETCH 响应的选项结构体。
type fetchWriterOptions struct {
	bodyStructure struct {
		extended    bool // 是否支持 BODYSTRUCTURE
		nonExtended bool // 是否支持 BODY
	}
	obsolete map[*imap.FetchItemBodySection]string // 存储过时的 FETCH 项目体部分
}

// handleFetch 处理 FETCH 命令。
//
// 参数：
//
//	dec - 解码器，用于解码 FETCH 请求。
//	numKind - 数字类型（UID 或其他）。
func (c *Conn) handleFetch(dec *imapwire.Decoder, numKind NumKind) error {
	var numSet imap.NumSet
	if !dec.ExpectSP() || !dec.ExpectNumSet(numKind.wire(), &numSet) || !dec.ExpectSP() {
		return dec.Err() // 期望的格式不正确，返回错误。
	}

	var options imap.FetchOptions
	writerOptions := fetchWriterOptions{obsolete: make(map[*imap.FetchItemBodySection]string)} // 初始化写入选项
	isList, err := dec.List(func() error {
		name, err := readFetchAttName(dec) // 读取 FETCH 属性名称
		if err != nil {
			return err
		}
		switch name {
		case "ALL", "FAST", "FULL":
			return newClientBugError("FETCH macros are not allowed in a list") // 不允许在列表中使用 FETCH 宏
		}
		return handleFetchAtt(dec, name, &options, &writerOptions) // 处理 FETCH 属性
	})
	if err != nil {
		return err
	}
	if !isList {
		name, err := readFetchAttName(dec) // 读取属性名称
		if err != nil {
			return err
		}

		// 处理宏
		switch name {
		case "ALL":
			options.Flags = true
			options.InternalDate = true
			options.RFC822Size = true
			options.Envelope = true
		case "FAST":
			options.Flags = true
			options.InternalDate = true
			options.RFC822Size = true
		case "FULL":
			options.Flags = true
			options.InternalDate = true
			options.RFC822Size = true
			options.Envelope = true
			handleFetchBodyStructure(&options, &writerOptions, false) // 处理邮件体结构
		default:
			if err := handleFetchAtt(dec, name, &options, &writerOptions); err != nil {
				return err
			}
		}
	}

	if !dec.ExpectCRLF() {
		return dec.Err() // 期望 CRLF 不正确，返回错误。
	}

	if err := c.checkState(imap.ConnStateSelected); err != nil { // 检查连接状态
		return err
	}

	if numKind == NumKindUID {
		options.UID = true // 如果是 UID 类型，设置 UID 选项为真。
	}

	w := &FetchWriter{conn: c, options: writerOptions}           // 创建 FetchWriter
	if err := c.session.Fetch(w, numSet, &options); err != nil { // 执行 FETCH 操作
		return err
	}
	return nil
}

// handleFetchAtt 处理 FETCH 属性。
//
// 参数：
//
//	dec - 解码器，用于解码 FETCH 请求。
//	attName - FETCH 属性名称。
//	options - FETCH 选项。
//	writerOptions - 写入选项。
func handleFetchAtt(dec *imapwire.Decoder, attName string, options *imap.FetchOptions, writerOptions *fetchWriterOptions) error {
	switch attName {
	case "BODYSTRUCTURE":
		handleFetchBodyStructure(options, writerOptions, true) // 处理 BODYSTRUCTURE
	case "ENVELOPE":
		options.Envelope = true // 设置 Envelope 选项为真
	case "FLAGS":
		options.Flags = true // 设置 Flags 选项为真
	case "INTERNALDATE":
		options.InternalDate = true // 设置 InternalDate 选项为真
	case "RFC822.SIZE":
		options.RFC822Size = true // 设置 RFC822.Size 选项为真
	case "UID":
		options.UID = true // 设置 UID 选项为真
	case "RFC822": // 等同于 BODY[]
		bs := &imap.FetchItemBodySection{}
		writerOptions.obsolete[bs] = attName                  // 记录过时的 FETCH 项目体部分
		options.BodySection = append(options.BodySection, bs) // 添加 BODY 部分
	case "RFC822.HEADER": // 等同于 BODY.PEEK[HEADER]
		bs := &imap.FetchItemBodySection{
			Specifier: imap.PartSpecifierHeader,
			Peek:      true, // 设置 Peek 为真
		}
		writerOptions.obsolete[bs] = attName                  // 记录过时的 FETCH 项目体部分
		options.BodySection = append(options.BodySection, bs) // 添加 HEADER 部分
	case "RFC822.TEXT": // 等同于 BODY[TEXT]
		bs := &imap.FetchItemBodySection{
			Specifier: imap.PartSpecifierText,
		}
		writerOptions.obsolete[bs] = attName                  // 记录过时的 FETCH 项目体部分
		options.BodySection = append(options.BodySection, bs) // 添加 TEXT 部分
	case "BINARY", "BINARY.PEEK":
		part, err := readSectionBinary(dec) // 读取二进制部分
		if err != nil {
			return err
		}
		partial, err := maybeReadPartial(dec) // 读取部分内容
		if err != nil {
			return err
		}
		bs := &imap.FetchItemBinarySection{
			Part:    part,
			Partial: partial,
			Peek:    attName == "BINARY.PEEK", // 判断是否为 BINARY.PEEK
		}
		options.BinarySection = append(options.BinarySection, bs) // 添加二进制部分
	case "BINARY.SIZE":
		part, err := readSectionBinary(dec) // 读取二进制部分
		if err != nil {
			return err
		}
		bss := &imap.FetchItemBinarySectionSize{Part: part}
		options.BinarySectionSize = append(options.BinarySectionSize, bss) // 添加二进制大小部分
	case "BODY":
		if !dec.Special('[') { // 检查是否为特殊字符 '['
			handleFetchBodyStructure(options, writerOptions, false) // 处理邮件体结构
			return nil
		}
		section := imap.FetchItemBodySection{}
		err := readSection(dec, &section) // 读取部分
		if err != nil {
			return err
		}
		section.Partial, err = maybeReadPartial(dec) // 读取部分内容
		if err != nil {
			return err
		}
		options.BodySection = append(options.BodySection, &section) // 添加 BODY 部分
	case "BODY.PEEK":
		if !dec.ExpectSpecial('[') { // 检查是否为特殊字符 '['
			return dec.Err()
		}
		section := imap.FetchItemBodySection{Peek: true} // 设置 Peek 为真
		err := readSection(dec, &section)                // 读取部分
		if err != nil {
			return err
		}
		section.Partial, err = maybeReadPartial(dec) // 读取部分内容
		if err != nil {
			return err
		}
		options.BodySection = append(options.BodySection, &section) // 添加 BODY 部分
	default:
		return newClientBugError("未知的 FETCH 数据项") // 返回未知 FETCH 数据项错误
	}
	return nil
}

// handleFetchBodyStructure 处理邮件体结构。
//
// 参数：
//
//	options - FETCH 选项。
//	writerOptions - 写入选项。
//	extended - 是否支持扩展结构。
func handleFetchBodyStructure(options *imap.FetchOptions, writerOptions *fetchWriterOptions, extended bool) {
	if options.BodyStructure == nil || extended {
		options.BodyStructure = &imap.FetchItemBodyStructure{Extended: extended} // 设置邮件体结构
	}
	if extended {
		writerOptions.bodyStructure.extended = true // 设置扩展标志
	} else {
		writerOptions.bodyStructure.nonExtended = true // 设置非扩展标志
	}
}

// readFetchAttName 读取 FETCH 属性名称。
//
// 参数：
//
//	dec - 解码器，用于解码 FETCH 请求。
//
// 返回：属性名称。
func readFetchAttName(dec *imapwire.Decoder) (string, error) {
	var attName string
	if !dec.Expect(dec.Func(&attName, isMsgAttNameChar), "msg-att name") { // 期望的格式不正确
		return "", dec.Err()
	}
	return strings.ToUpper(attName), nil // 返回大写的属性名称
}

// isMsgAttNameChar 判断字符是否为有效的消息属性名称字符。
func isMsgAttNameChar(ch byte) bool {
	return ch != '[' && imapwire.IsAtomChar(ch) // 检查字符是否为有效的原子字符
}

// readSection 读取部分信息。
//
// 参数：
//
//	dec - 解码器，用于解码部分信息。
//	section - FETCH 项目体部分。
func readSection(dec *imapwire.Decoder, section *imap.FetchItemBodySection) error {
	if dec.Special(']') { // 检查是否为特殊字符 ']'
		return nil
	}

	var dot bool
	section.Part, dot = readSectionPart(dec) // 读取部分
	if dot || len(section.Part) == 0 {
		var specifier string
		if dot {
			if !dec.ExpectAtom(&specifier) { // 期望的格式不正确
				return dec.Err()
			}
		} else {
			dec.Atom(&specifier) // 读取原子字符串
		}

		switch specifier := imap.PartSpecifier(strings.ToUpper(specifier)); specifier {
		case imap.PartSpecifierNone, imap.PartSpecifierHeader, imap.PartSpecifierMIME, imap.PartSpecifierText:
			section.Specifier = specifier // 设置部分说明符
		case "HEADER.FIELDS", "HEADER.FIELDS.NOT":
			if !dec.ExpectSP() { // 期望空格
				return dec.Err()
			}
			var err error
			headerList, err := readHeaderList(dec) // 读取头部字段列表
			if err != nil {
				return err
			}
			section.Specifier = imap.PartSpecifierHeader // 设置部分说明符为 HEADER
			if specifier == "HEADER.FIELDS" {
				section.HeaderFields = headerList // 设置头部字段
			} else {
				section.HeaderFieldsNot = headerList // 设置不包含的头部字段
			}
		default:
			return newClientBugError("未知的正文部分说明符") // 返回未知部分说明符错误
		}
	}

	if !dec.ExpectSpecial(']') { // 检查是否为特殊字符 ']'
		return dec.Err()
	}

	return nil
}

// readSectionPart 读取部分的序号。
func readSectionPart(dec *imapwire.Decoder) (part []int, dot bool) {
	for {
		dot = len(part) > 0           // 判断是否已经有部分
		if dot && !dec.Special('.') { // 检查是否为特殊字符 '.'
			return part, false
		}

		var num uint32
		if !dec.Number(&num) { // 读取数字
			return part, dot
		}
		part = append(part, int(num)) // 添加数字到部分列表
	}
}

// readHeaderList 读取头部字段列表。
//
// 参数：
//
//	dec - 解码器，用于解码头部字段。
//
// 返回：头部字段列表。
func readHeaderList(dec *imapwire.Decoder) ([]string, error) {
	var l []string
	err := dec.ExpectList(func() error {
		var s string
		if !dec.ExpectAString(&s) { // 期望的格式不正确
			return dec.Err()
		}
		l = append(l, s) // 添加头部字段到列表
		return nil
	})
	return l, err
}

// readSectionBinary 读取二进制部分。
//
// 参数：
//
//	dec - 解码器，用于解码二进制部分。
//
// 返回：二进制部分的序号。
func readSectionBinary(dec *imapwire.Decoder) ([]int, error) {
	if !dec.ExpectSpecial('[') { // 检查是否为特殊字符 '['
		return nil, dec.Err()
	}
	if dec.Special(']') { // 检查是否为特殊字符 ']'
		return nil, nil
	}

	var l []int
	for {
		var num uint32
		if !dec.ExpectNumber(&num) { // 读取数字
			return l, dec.Err()
		}
		l = append(l, int(num)) // 添加数字到列表

		if !dec.Special('.') { // 检查是否为特殊字符 '.'
			break
		}
	}

	if !dec.ExpectSpecial(']') { // 检查是否为特殊字符 ']'
		return l, dec.Err()
	}
	return l, nil
}

// maybeReadPartial 可能读取部分信息。
//
// 参数：
//
//	dec - 解码器，用于解码部分信息。
//
// 返回：部分信息。
func maybeReadPartial(dec *imapwire.Decoder) (*imap.SectionPartial, error) {
	if !dec.Special('<') { // 检查是否为特殊字符 '<'
		return nil, nil
	}
	var partial imap.SectionPartial
	if !dec.ExpectNumber64(&partial.Offset) || !dec.ExpectSpecial('.') || !dec.ExpectNumber64(&partial.Size) || !dec.ExpectSpecial('>') {
		return nil, dec.Err() // 返回错误
	}
	return &partial, nil
}

// FetchWriter 写入 FETCH 响应。
type FetchWriter struct {
	conn    *Conn              // 连接对象
	options fetchWriterOptions // 写入选项
}

// CreateMessage 为消息写入 FETCH 响应。
//
// FetchResponseWriter.Close 必须在写入任何更多消息数据项之前调用。
func (cmd *FetchWriter) CreateMessage(seqNum uint32) *FetchResponseWriter {
	enc := newResponseEncoder(cmd.conn) // 创建响应编码器
	enc.Atom("*").SP().Number(seqNum).SP().Atom("FETCH").SP().Special('(')
	return &FetchResponseWriter{enc: enc, options: cmd.options} // 返回 FETCH 响应写入器
}

// FetchResponseWriter 为消息写入单个 FETCH 响应。
type FetchResponseWriter struct {
	enc     *responseEncoder   // 响应编码器
	options fetchWriterOptions // 写入选项

	hasItem bool // 是否已经写入项
}

// writeItemSep 写入项分隔符。
func (w *FetchResponseWriter) writeItemSep() {
	if w.hasItem { // 如果已写入项，则添加空格
		w.enc.SP()
	}
	w.hasItem = true // 标记已写入项
}

// WriteUID 写入消息的 UID。
func (w *FetchResponseWriter) WriteUID(uid imap.UID) {
	w.writeItemSep()                // 写入分隔符
	w.enc.Atom("UID").SP().UID(uid) // 写入 UID
}

// WriteFlags 写入消息的标志。
func (w *FetchResponseWriter) WriteFlags(flags []imap.Flag) {
	w.writeItemSep() // 写入分隔符
	w.enc.Atom("FLAGS").SP().List(len(flags), func(i int) {
		w.enc.Flag(flags[i]) // 写入每个标志
	})
}

// WriteRFC822Size 写入消息的完整大小。
func (w *FetchResponseWriter) WriteRFC822Size(size int64) {
	w.writeItemSep()                              // 写入分隔符
	w.enc.Atom("RFC822.SIZE").SP().Number64(size) // 写入 RFC822.SIZE
}

// WriteInternalDate 写入消息的内部日期。
func (w *FetchResponseWriter) WriteInternalDate(t time.Time) {
	w.writeItemSep()                                                          // 写入分隔符
	w.enc.Atom("INTERNALDATE").SP().String(t.Format(internal.DateTimeLayout)) // 写入内部日期
}

// WriteBodySection 写入邮件体部分。
//
// 返回的 io.WriteCloser 必须在写入任何更多消息数据项之前关闭。
func (w *FetchResponseWriter) WriteBodySection(section *imap.FetchItemBodySection, size int64) io.WriteCloser {
	w.writeItemSep() // 写入分隔符
	enc := w.enc.Encoder

	if obs, ok := w.options.obsolete[section]; ok { // 检查是否为过时部分
		enc.Atom(obs) // 写入过时的属性名称
	} else {
		writeItemBodySection(enc, section) // 写入邮件体部分
	}

	enc.SP()                   // 添加空格
	return w.enc.Literal(size) // 返回字面量写入器
}

// writeItemBodySection 编写 BODY 部分的编码方法。
//
// enc: 用于编码的 imapwire.Encoder。
// section: 要编码的 imap.FetchItemBodySection，包含部分和说明。
func writeItemBodySection(enc *imapwire.Encoder, section *imap.FetchItemBodySection) {
	enc.Atom("BODY")                    // 写入 "BODY" 原子
	enc.Special('[')                    // 开始一个特殊字符 '['
	writeSectionPart(enc, section.Part) // 写入部分信息
	if len(section.Part) > 0 && section.Specifier != imap.PartSpecifierNone {
		enc.Special('.') // 如果部分非空且有说明，写入特殊字符 '.'
	}
	if section.Specifier != imap.PartSpecifierNone {
		enc.Atom(string(section.Specifier)) // 写入部分说明

		var headerList []string
		if len(section.HeaderFields) > 0 {
			headerList = section.HeaderFields
			enc.Atom(".FIELDS") // 写入 ".FIELDS" 表示有头部字段
		} else if len(section.HeaderFieldsNot) > 0 {
			headerList = section.HeaderFieldsNot
			enc.Atom(".FIELDS.NOT") // 写入 ".FIELDS.NOT" 表示没有头部字段
		}

		if len(headerList) > 0 {
			enc.SP().List(len(headerList), func(i int) {
				enc.String(headerList[i]) // 写入每个头部字段
			})
		}
	}
	enc.Special(']') // 结束特殊字符 ']'
	if partial := section.Partial; partial != nil {
		enc.Special('<').Number(uint32(partial.Offset)).Special('>') // 写入偏移信息
	}
}

// WriteBinarySection 写入二进制部分的方法。
//
// section: 要编码的 imap.FetchItemBinarySection。
// size: 二进制数据的大小。
func (w *FetchResponseWriter) WriteBinarySection(section *imap.FetchItemBinarySection, size int64) io.WriteCloser {
	w.writeItemSep()     // 写入项分隔符
	enc := w.enc.Encoder // 获取编码器

	enc.Atom("BINARY").Special('[')     // 写入 "BINARY" 原子
	writeSectionPart(enc, section.Part) // 写入部分信息
	enc.Special(']').SP()               // 结束特殊字符 ']' 并添加空格
	enc.Special('~')                    // 指示字面值类型为 8
	return w.enc.Literal(size)          // 返回一个写入器，用于写入二进制数据
}

// WriteEnvelope 写入消息的信封。
//
// envelope: 要编码的 imap.Envelope，包含邮件的信封信息。
func (w *FetchResponseWriter) WriteEnvelope(envelope *imap.Envelope) {
	w.writeItemSep()             // 写入项分隔符
	enc := w.enc.Encoder         // 获取编码器
	enc.Atom("ENVELOPE").SP()    // 写入 "ENVELOPE" 原子并添加空格
	writeEnvelope(enc, envelope) // 编写信封信息
}

// WriteBodyStructure 写入消息的主体结构（可以是 BODYSTRUCTURE 或 BODY）。
//
// bs: 消息的主体结构，可以是不同类型的主体结构。
func (w *FetchResponseWriter) WriteBodyStructure(bs imap.BodyStructure) {
	if w.options.bodyStructure.nonExtended {
		w.writeBodyStructure(bs, false) // 如果不是扩展模式，写入主体结构
	}

	if w.options.bodyStructure.extended {
		var isExtended bool
		switch bs := bs.(type) {
		case *imap.BodyStructureSinglePart:
			isExtended = bs.Extended != nil // 检查是否有扩展信息
		case *imap.BodyStructureMultiPart:
			isExtended = bs.Extended != nil // 检查是否有扩展信息
		}
		if !isExtended {
			panic("imapserver: 客户端请求扩展的主体结构，但返回的是非扩展的主体结构。") // 如果客户端请求扩展结构，但未写入扩展，抛出错误
		}

		w.writeBodyStructure(bs, true) // 如果是扩展模式，写入主体结构
	}
}

// writeBodyStructure 编写主体结构的方法。
//
// bs: 消息的主体结构。
// extended: 是否为扩展模式。
func (w *FetchResponseWriter) writeBodyStructure(bs imap.BodyStructure, extended bool) {
	item := "BODY"
	if extended {
		item = "BODYSTRUCTURE" // 根据模式选择写入 "BODY" 或 "BODYSTRUCTURE"
	}

	w.writeItemSep()                      // 写入项分隔符
	enc := w.enc.Encoder                  // 获取编码器
	enc.Atom(item).SP()                   // 写入主体标识并添加空格
	writeBodyStructure(enc, bs, extended) // 编写主体结构
}

// Close 关闭 FETCH 消息编写器的方法。
func (w *FetchResponseWriter) Close() error {
	if w.enc == nil {
		return fmt.Errorf("imapserver: FetchResponseWriter 已经关闭。") // 如果已经关闭，返回错误
	}
	err := w.enc.Special(')').CRLF() // 写入特殊字符 ')' 并换行
	w.enc.end()                      // 结束编码
	w.enc = nil                      // 清空编码器
	return err                       // 返回可能的错误
}

// writeEnvelope 编写邮件信封的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// envelope: 要编码的 imap.Envelope，包含邮件的信封信息。
func writeEnvelope(enc *imapwire.Encoder, envelope *imap.Envelope) {
	if envelope == nil {
		envelope = new(imap.Envelope) // 如果 envelope 为 nil，创建新的 Envelope 实例
	}

	sender := envelope.Sender // 获取发件人
	if sender == nil {
		sender = envelope.From // 如果发件人为空，使用 From 字段
	}
	replyTo := envelope.ReplyTo // 获取回复地址
	if replyTo == nil {
		replyTo = envelope.From // 如果回复地址为空，使用 From 字段
	}

	enc.Special('(') // 开始一个特殊字符 '('
	if envelope.Date.IsZero() {
		enc.NIL() // 如果日期为空，写入 nil
	} else {
		enc.String(envelope.Date.Format(envelopeDateLayout)) // 写入格式化的日期
	}
	enc.SP()                                                            // 添加空格
	writeNString(enc, mime.QEncoding.Encode("utf-8", envelope.Subject)) // 写入主题
	addrs := [][]imap.Address{
		envelope.From,
		sender,
		replyTo,
		envelope.To,
		envelope.Cc,
		envelope.Bcc,
	} // 收集邮件地址列表
	for _, l := range addrs {
		enc.SP()                 // 添加空格
		writeAddressList(enc, l) // 编写地址列表
	}
	enc.SP() // 添加空格
	if len(envelope.InReplyTo) > 0 {
		enc.String("<" + strings.Join(envelope.InReplyTo, "> <") + ">") // 写入 In-Reply-To 地址
	} else {
		enc.NIL() // 如果没有 In-Reply-To 地址，写入 nil
	}
	enc.SP() // 添加空格
	if envelope.MessageID != "" {
		enc.String("<" + envelope.MessageID + ">") // 写入 Message-ID
	} else {
		enc.NIL() // 如果没有 Message-ID，写入 nil
	}
	enc.Special(')') // 结束特殊字符 ')'
}

// writeAddressList 编写地址列表的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// l: 地址列表。
func writeAddressList(enc *imapwire.Encoder, l []imap.Address) {
	if l == nil {
		enc.NIL() // 如果地址列表为 nil，写入 nil
		return
	}

	enc.List(len(l), func(i int) {
		addr := l[i]                                                 // 获取地址
		enc.Special('(')                                             // 开始一个特殊字符 '('
		writeNString(enc, mime.QEncoding.Encode("utf-8", addr.Name)) // 写入地址名称
		enc.SP().NIL().SP()                                          // 添加空格和 nil
		writeNString(enc, addr.Mailbox)                              // 写入邮箱名
		enc.SP()                                                     // 添加空格
		writeNString(enc, addr.Host)                                 // 写入主机名
		enc.Special(')')                                             // 结束特殊字符 ')'
	})
}

// writeNString 编写可能为 nil 的字符串的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// s: 要写入的字符串。
func writeNString(enc *imapwire.Encoder, s string) {
	if s == "" {
		enc.NIL() // 如果字符串为空，写入 nil
	} else {
		enc.String(s) // 否则写入字符串
	}
}

// writeSectionPart 编写部分信息的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// part: 部分索引列表。
func writeSectionPart(enc *imapwire.Encoder, part []int) {
	if len(part) == 0 {
		return // 如果部分为空，则直接返回
	}

	var l []string
	for _, num := range part {
		l = append(l, fmt.Sprintf("%v", num)) // 将部分索引转换为字符串
	}
	enc.Atom(strings.Join(l, ".")) // 使用 '.' 连接索引并写入
}

// writeBodyStructure 编写主体结构的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// bs: 消息的主体结构。
// extended: 是否为扩展模式。
func writeBodyStructure(enc *imapwire.Encoder, bs imap.BodyStructure, extended bool) {
	enc.Special('(') // 开始一个特殊字符 '('
	switch bs := bs.(type) {
	case *imap.BodyStructureSinglePart:
		writeBodyType1part(enc, bs, extended) // 写入单一部分的主体结构
	case *imap.BodyStructureMultiPart:
		writeBodyTypeMpart(enc, bs, extended) // 写入多部分的主体结构
	default:
		panic(fmt.Errorf("未知的正文结构类型 %T", bs)) // 如果未知的主体结构类型，抛出错误
	}
	enc.Special(')') // 结束特殊字符 ')'
}

// writeBodyType1part 编写单一部分的主体结构的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// bs: 单一部分的主体结构。
// extended: 是否为扩展模式。
func writeBodyType1part(enc *imapwire.Encoder, bs *imap.BodyStructureSinglePart, extended bool) {
	enc.String(bs.Type).SP().String(bs.Subtype).SP() // 写入主体类型和子类型并添加空格
	writeBodyFldParam(enc, bs.Params)                // 编写参数
	enc.SP()                                         // 添加空格
	writeNString(enc, bs.ID)                         // 编写 ID
	enc.SP()                                         // 添加空格
	writeNString(enc, bs.Description)                // 编写描述
	enc.SP()                                         // 添加空格
	if bs.Encoding == "" {
		enc.String("7BIT") // 如果编码为空，默认写入 "7BIT"
	} else {
		enc.String(strings.ToUpper(bs.Encoding)) // 否则写入编码类型
	}
	enc.SP().Number(bs.Size) // 添加空格并写入大小

	if msg := bs.MessageRFC822; msg != nil {
		enc.SP()                                             // 添加空格
		writeEnvelope(enc, msg.Envelope)                     // 写入嵌套的消息信封
		enc.SP()                                             // 添加空格
		writeBodyStructure(enc, msg.BodyStructure, extended) // 写入嵌套的主体结构
		enc.SP().Number64(msg.NumLines)                      // 添加空格并写入行数
	} else if text := bs.Text; text != nil {
		enc.SP().Number64(text.NumLines) // 如果存在文本，添加空格并写入行数
	}

	if !extended {
		return // 如果不是扩展模式，直接返回
	}
	ext := bs.Extended // 获取扩展信息

	enc.SP()                              // 添加空格
	enc.NIL()                             // MD5
	enc.SP()                              // 添加空格
	writeBodyFldDsp(enc, ext.Disposition) // 编写处理信息
	enc.SP()                              // 添加空格
	writeBodyFldLang(enc, ext.Language)   // 编写语言信息
	enc.SP()                              // 添加空格
	writeNString(enc, ext.Location)       // 编写位置
}

// writeBodyTypeMpart 编写多部分的主体结构的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// bs: 多部分的主体结构。
// extended: 是否为扩展模式。
func writeBodyTypeMpart(enc *imapwire.Encoder, bs *imap.BodyStructureMultiPart, extended bool) {
	if len(bs.Children) == 0 {
		panic("“imapserver：imap.BodyStructureMultiPart 必须至少有一个子项") // 如果没有子部分，抛出错误
	}
	for i, child := range bs.Children {
		if i > 0 {
			enc.SP() // 添加空格
		}
		writeBodyStructure(enc, child, extended) // 编写子部分的主体结构
	}

	enc.SP().String(bs.Subtype) // 添加空格并写入子类型

	if !extended {
		return // 如果不是扩展模式，直接返回
	}
	ext := bs.Extended // 获取扩展信息

	enc.SP()                              // 添加空格
	writeBodyFldParam(enc, ext.Params)    // 编写参数
	enc.SP()                              // 添加空格
	writeBodyFldDsp(enc, ext.Disposition) // 编写处理信息
	enc.SP()                              // 添加空格
	writeBodyFldLang(enc, ext.Language)   // 编写语言信息
	enc.SP()                              // 添加空格
	writeNString(enc, ext.Location)       // 编写位置
}

// writeBodyFldParam 编写主体字段参数的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// params: 字段参数。
func writeBodyFldParam(enc *imapwire.Encoder, params map[string]string) {
	if params == nil {
		enc.NIL() // 如果参数为 nil，写入 nil
		return
	}

	var l []string
	for k := range params {
		l = append(l, k) // 收集参数键
	}
	sort.Strings(l) // 对键进行排序

	enc.List(len(l), func(i int) {
		k := l[i]                    // 获取键
		v := params[k]               // 获取值
		enc.String(k).SP().String(v) // 写入键值对
	})
}

// writeBodyFldDsp 编写主体字段处理信息的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// disp: 处理信息。
func writeBodyFldDsp(enc *imapwire.Encoder, disp *imap.BodyStructureDisposition) {
	if disp == nil {
		enc.NIL() // 如果处理信息为 nil，写入 nil
		return
	}

	enc.Special('(').String(disp.Value).SP() // 开始 '('，写入处理值并添加空格
	writeBodyFldParam(enc, disp.Params)      // 编写参数
	enc.Special(')')                         // 结束特殊字符 ')'
}

// writeBodyFldLang 编写主体字段语言信息的方法。
//
// enc: 用于编码的 imapwire.Encoder。
// l: 语言列表。
func writeBodyFldLang(enc *imapwire.Encoder, l []string) {
	if l == nil {
		enc.NIL() // 如果语言列表为 nil，写入 nil
	} else {
		enc.List(len(l), func(i int) {
			enc.String(l[i]) // 写入每种语言
		})
	}
}
