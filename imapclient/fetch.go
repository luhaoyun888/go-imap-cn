package imapclient

import (
	"fmt"
	"io"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// Fetch 发送一个FETCH命令。
// 调用者必须完全消费 FetchCommand。一个简单的方式是使用 defer 调用 FetchCommand.Close 来确保。
// 如果 options 指针为 nil，相当于一个默认的 options 值。
func (c *Client) Fetch(numSet imap.NumSet, options *imap.FetchOptions) *FetchCommand {
	if options == nil {
		options = new(imap.FetchOptions)
	}

	// 获取数字集合类型
	numKind := imapwire.NumSetKind(numSet)

	// 初始化 FetchCommand 并创建消息通道
	cmd := &FetchCommand{
		numSet: numSet,
		msgs:   make(chan *FetchMessageData, 128),
	}

	// 开始一个 FETCH 命令的编码
	enc := c.beginCommand(uidCmdName("FETCH", numKind), cmd)

	// 编码命令中的数字集合
	enc.SP().NumSet(numSet).SP()
	// 写入FETCH请求的项目
	writeFetchItems(enc.Encoder, numKind, options)
	// 如果有 CHANGEDSINCE 选项，添加到命令中
	if options.ChangedSince != 0 {
		enc.SP().Special('(').Atom("已更改自").SP().ModSeq(options.ChangedSince).Special(')')
	}
	// 结束命令编码
	enc.end()
	return cmd
}

// writeFetchItems 写入 FETCH 命令中的各项请求
// 参数说明：
// enc 是一个命令的编码器
// numKind 是数字集合的类型
// options 是FETCH请求的选项
func writeFetchItems(enc *imapwire.Encoder, numKind imapwire.NumKind, options *imap.FetchOptions) {
	listEnc := enc.BeginList()

	// 如果请求 UID FETCH，则确保第一个项目请求 UID
	if options.UID || numKind == imapwire.NumKindUID {
		listEnc.Item().Atom("唯一标识符")
	}

	// 根据请求选项，将对应的项目加入到FETCH命令中
	m := map[string]bool{
		"正文":        options.BodyStructure != nil && !options.BodyStructure.Extended,
		"完整结构":      options.BodyStructure != nil && options.BodyStructure.Extended,
		"信封":        options.Envelope,
		"标志":        options.Flags,
		"内部日期":      options.InternalDate,
		"RFC822.大小": options.RFC822Size,
		"修改序列号":     options.ModSeq,
	}
	for k, req := range m {
		if req {
			listEnc.Item().Atom(k)
		}
	}

	// 写入请求的正文部分和二进制部分
	for _, bs := range options.BodySection {
		writeFetchItemBodySection(listEnc.Item(), bs)
	}
	for _, bs := range options.BinarySection {
		writeFetchItemBinarySection(listEnc.Item(), bs)
	}
	for _, bss := range options.BinarySectionSize {
		writeFetchItemBinarySectionSize(listEnc.Item(), bss)
	}

	listEnc.End()
}

// writeFetchItemBodySection 写入 FETCH BODY[] 请求
// 参数说明：
// enc 是命令的编码器
// item 是请求的正文部分
func writeFetchItemBodySection(enc *imapwire.Encoder, item *imap.FetchItemBodySection) {
	enc.Atom("BODY")
	if item.Peek {
		enc.Atom(".PEEK")
	}
	enc.Special('[')
	writeSectionPart(enc, item.Part)
	if len(item.Part) > 0 && item.Specifier != imap.PartSpecifierNone {
		enc.Special('.')
	}
	if item.Specifier != imap.PartSpecifierNone {
		enc.Atom(string(item.Specifier))

		var headerList []string
		if len(item.HeaderFields) > 0 {
			headerList = item.HeaderFields
			enc.Atom(".FIELDS")
		} else if len(item.HeaderFieldsNot) > 0 {
			headerList = item.HeaderFieldsNot
			enc.Atom(".FIELDS.NOT")
		}

		if len(headerList) > 0 {
			enc.SP().List(len(headerList), func(i int) {
				enc.String(headerList[i])
			})
		}
	}
	enc.Special(']')
	writeSectionPartial(enc, item.Partial)
}

// writeFetchItemBinarySection 写入 FETCH BINARY[] 请求
// 参数说明：
// enc 是命令的编码器
// item 是请求的二进制部分
func writeFetchItemBinarySection(enc *imapwire.Encoder, item *imap.FetchItemBinarySection) {
	enc.Atom("二进制")
	if item.Peek {
		enc.Atom(".窥视")
	}
	enc.Special('[')
	writeSectionPart(enc, item.Part)
	enc.Special(']')
	writeSectionPartial(enc, item.Partial)
}

// writeFetchItemBinarySectionSize 写入 FETCH BINARY.SIZE[] 请求
// 参数说明：
// enc 是命令的编码器
// item 是请求的二进制大小部分
func writeFetchItemBinarySectionSize(enc *imapwire.Encoder, item *imap.FetchItemBinarySectionSize) {
	enc.Atom("二进制.大小")
	enc.Special('[')
	writeSectionPart(enc, item.Part)
	enc.Special(']')
}

// writeSectionPart 写入部分节的数字序列
// 参数说明：
// enc 是命令的编码器
// part 是节的编号列表
func writeSectionPart(enc *imapwire.Encoder, part []int) {
	if len(part) == 0 {
		return
	}

	var l []string
	for _, num := range part {
		l = append(l, fmt.Sprintf("%v", num))
	}
	enc.Atom(strings.Join(l, "."))
}

// writeSectionPartial 写入部分节的偏移和大小
// 参数说明：
// enc 是命令的编码器
// partial 是部分节的偏移和大小信息
func writeSectionPartial(enc *imapwire.Encoder, partial *imap.SectionPartial) {
	if partial == nil {
		return
	}
	enc.Special('<').Number64(partial.Offset).Special('.').Number64(partial.Size).Special('>')
}

// FetchCommand 表示一个 FETCH 命令。
type FetchCommand struct {
	commandBase

	// numSet 是用于标识消息的数值集合，可能是顺序集合或 UID 集合。
	numSet imap.NumSet
	// recvSeqSet 用于接收的顺序号集合。
	recvSeqSet imap.SeqSet
	// recvUIDSet 用于接收的 UID 集合。
	recvUIDSet imap.UIDSet

	// msgs 是用于存储 FETCH 消息数据的通道。
	msgs chan *FetchMessageData
	// prev 保存上一个 FETCH 消息数据。
	prev *FetchMessageData
}

// recvSeqNum 接收顺序号。
// 参数 seqNum 是顺序号。
// 返回值表示是否成功接收。
func (cmd *FetchCommand) recvSeqNum(seqNum uint32) bool {
	// 检查 numSet 是否为顺序集合并且包含 seqNum。
	set, ok := cmd.numSet.(imap.SeqSet)
	if !ok || !set.Contains(seqNum) {
		return false
	}

	// 检查 recvSeqSet 是否已经包含 seqNum。
	if cmd.recvSeqSet.Contains(seqNum) {
		return false
	}

	// 添加 seqNum 到 recvSeqSet。
	cmd.recvSeqSet.AddNum(seqNum)
	return true
}

// recvUID 接收 UID。
// 参数 uid 是消息的唯一标识符。
// 返回值表示是否成功接收。
func (cmd *FetchCommand) recvUID(uid imap.UID) bool {
	// 检查 numSet 是否为 UID 集合并且包含 uid。
	set, ok := cmd.numSet.(imap.UIDSet)
	if !ok || !set.Contains(uid) {
		return false
	}

	// 检查 recvUIDSet 是否已经包含 uid。
	if cmd.recvUIDSet.Contains(uid) {
		return false
	}

	// 添加 uid 到 recvUIDSet。
	cmd.recvUIDSet.AddNum(uid)
	return true
}

// Next 读取下一条消息。
// 如果成功，返回消息；如果出错或没有更多消息，返回 nil。
// 要检查错误值，请使用 Close。
func (cmd *FetchCommand) Next() *FetchMessageData {
	if cmd.prev != nil {
		cmd.prev.discard()
	}
	// 读取下一条消息。
	cmd.prev = <-cmd.msgs
	return cmd.prev
}

// Close 关闭命令。
// 调用 Close 会解除阻塞的 IMAP 客户端解码器，并让它读取下一条响应。
// 在 Close 之后，Next 将始终返回 nil。
func (cmd *FetchCommand) Close() error {
	for cmd.Next() != nil {
		// 忽略
	}
	return cmd.wait()
}

// Collect 收集消息数据到列表中。
// 此方法将读取并将消息内容存储在内存中。对于合理大小的消息内容，这是可接受的，但对于如附件等大文件，可能不合适。
// 该方法等效于反复调用 Next 然后 Close。
func (cmd *FetchCommand) Collect() ([]*FetchMessageBuffer, error) {
	defer cmd.Close()

	var l []*FetchMessageBuffer
	for {
		// 读取下一条消息。
		msg := cmd.Next()
		if msg == nil {
			break
		}

		// 收集消息内容。
		buf, err := msg.Collect()
		if err != nil {
			return l, err
		}

		l = append(l, buf)
	}
	return l, cmd.Close()
}

// FetchMessageData 包含消息的 FETCH 数据。
type FetchMessageData struct {
	// SeqNum 是消息的顺序号。
	SeqNum uint32

	// items 是 FETCH 项数据的通道。
	items chan FetchItemData
	// prev 保存上一个 FETCH 项数据。
	prev FetchItemData
}

// Next 读取下一条数据项。
// 如果还有数据项，返回下一条数据项；否则返回 nil。
func (data *FetchMessageData) Next() FetchItemData {
	if d, ok := data.prev.(discarder); ok {
		d.discard()
	}

	// 读取下一条数据项。
	item := <-data.items
	data.prev = item
	return item
}

// discard 丢弃所有剩余的数据项。
func (data *FetchMessageData) discard() {
	for {
		if item := data.Next(); item == nil {
			break
		}
	}
}

// Collect 收集消息数据到结构体中。
// 此方法将读取并将消息内容存储在内存中。对于合理大小的消息内容，这是可接受的，但对于如附件等大文件，可能不合适。
func (data *FetchMessageData) Collect() (*FetchMessageBuffer, error) {
	defer data.discard()

	// 创建一个消息缓冲区，并将 SeqNum 赋值。
	buf := &FetchMessageBuffer{SeqNum: data.SeqNum}
	for {
		// 读取下一条数据项。
		item := data.Next()
		if item == nil {
			break
		}
		// 填充数据项到缓冲区。
		if err := buf.populateItemData(item); err != nil {
			return buf, err
		}
	}
	return buf, nil
}

// FetchItemData 表示消息的 FETCH 项数据。
type FetchItemData interface {
	// fetchItemData 用于标记接口实现。
	fetchItemData()
}

var (
	_ FetchItemData = FetchItemDataBodySection{}
	_ FetchItemData = FetchItemDataBinarySection{}
	_ FetchItemData = FetchItemDataFlags{}
	_ FetchItemData = FetchItemDataEnvelope{}
	_ FetchItemData = FetchItemDataInternalDate{}
	_ FetchItemData = FetchItemDataRFC822Size{}
	_ FetchItemData = FetchItemDataUID{}
	_ FetchItemData = FetchItemDataBodyStructure{}
)

// discarder 表示可以丢弃的接口。
type discarder interface {
	discard()
}

var (
	_ discarder = FetchItemDataBodySection{}
	_ discarder = FetchItemDataBinarySection{}
)

// FetchItemDataBodySection 保存 FETCH BODY[] 返回的数据。
// Literal 可能为空。
type FetchItemDataBodySection struct {
	// Section 表示 FETCH 项的 BODY 部分。
	Section *imap.FetchItemBodySection
	// Literal 表示数据内容的读取器。
	Literal imap.LiteralReader
}

func (FetchItemDataBodySection) fetchItemData() {}

// discard 丢弃未读取的数据。
func (item FetchItemDataBodySection) discard() {
	if item.Literal != nil {
		io.Copy(io.Discard, item.Literal) // 丢弃未使用的字节。
	}
}

// FetchItemDataBinarySection 保存 FETCH BINARY[] 返回的数据。
// Literal 可能为空。
type FetchItemDataBinarySection struct {
	// Section 表示 FETCH 项的 BINARY 部分。
	Section *imap.FetchItemBinarySection
	// Literal 表示数据内容的读取器。
	Literal imap.LiteralReader
}

func (FetchItemDataBinarySection) fetchItemData() {}

// discard 丢弃未读取的数据。
func (item FetchItemDataBinarySection) discard() {
	if item.Literal != nil {
		io.Copy(io.Discard, item.Literal) // 丢弃未使用的字节。
	}
}

// FetchItemDataFlags 保存 FETCH FLAGS 返回的数据。
type FetchItemDataFlags struct {
	// Flags 是消息的标志列表。
	Flags []imap.Flag
}

func (FetchItemDataFlags) fetchItemData() {}

// FetchItemDataEnvelope 保存 FETCH ENVELOPE 返回的数据。
type FetchItemDataEnvelope struct {
	// Envelope 是消息的信封数据。
	Envelope *imap.Envelope
}

func (FetchItemDataEnvelope) fetchItemData() {}

// FetchItemDataInternalDate 保存 FETCH INTERNALDATE 返回的数据。
type FetchItemDataInternalDate struct {
	// Time 是消息的内部时间。
	Time time.Time
}

func (FetchItemDataInternalDate) fetchItemData() {}

// FetchItemDataRFC822Size 保存 FETCH RFC822.SIZE 返回的数据。
type FetchItemDataRFC822Size struct {
	// Size 是消息的大小。
	Size int64
}

func (FetchItemDataRFC822Size) fetchItemData() {}

// FetchItemDataUID 保存 FETCH UID 返回的数据。
type FetchItemDataUID struct {
	// UID 是消息的唯一标识符。
	UID imap.UID
}

func (FetchItemDataUID) fetchItemData() {}

// FetchItemDataBodyStructure 保存 FETCH BODYSTRUCTURE 或 FETCH BODY 返回的数据。
type FetchItemDataBodyStructure struct {
	// BodyStructure 是消息的正文结构。
	BodyStructure imap.BodyStructure
	// IsExtended 指示是否为扩展的 BODYSTRUCTURE。
	IsExtended bool
}

func (FetchItemDataBodyStructure) fetchItemData() {}

// FetchItemDataBinarySectionSize 保存 FETCH BINARY.SIZE[] 返回的数据。
type FetchItemDataBinarySectionSize struct {
	// Part 是消息的部分标识符。
	Part []int
	// Size 是部分的大小。
	Size uint32
}

func (FetchItemDataBinarySectionSize) fetchItemData() {}

// FetchItemDataModSeq 保存 FETCH MODSEQ 返回的数据。
// 需要 CONDSTORE 扩展。
type FetchItemDataModSeq struct {
	// ModSeq 是消息的修订序号。
	ModSeq uint64
}

func (FetchItemDataModSeq) fetchItemData() {}

// FetchMessageBuffer 是一个用于存储 FetchMessageData 返回数据的缓冲区结构体。
//
// SeqNum 字段始终会被填充。其他字段都是可选的。
type FetchMessageBuffer struct {
	SeqNum            uint32                                  // 序列号
	Flags             []imap.Flag                             // 标志
	Envelope          *imap.Envelope                          // 邮件封套
	InternalDate      time.Time                               // 内部日期
	RFC822Size        int64                                   // 邮件大小
	UID               imap.UID                                // 邮件唯一标识
	BodyStructure     imap.BodyStructure                      // 邮件正文结构
	BodySection       map[*imap.FetchItemBodySection][]byte   // 正文部分
	BinarySection     map[*imap.FetchItemBinarySection][]byte // 二进制部分
	BinarySectionSize []FetchItemDataBinarySectionSize        // 二进制部分大小
	ModSeq            uint64                                  // 修改序列号 (需要 CONDSTORE 支持)
}

// populateItemData 根据提供的 FetchItemData 数据填充对应的字段。
// 参数:
//
//	item: FetchItemData 类型，代表要填充的提取项数据。
func (buf *FetchMessageBuffer) populateItemData(item FetchItemData) error {
	switch item := item.(type) {
	case FetchItemDataBodySection:
		var b []byte
		if item.Literal != nil {
			var err error
			b, err = io.ReadAll(item.Literal)
			if err != nil {
				return err
			}
		}
		if buf.BodySection == nil {
			buf.BodySection = make(map[*imap.FetchItemBodySection][]byte)
		}
		buf.BodySection[item.Section] = b
	case FetchItemDataBinarySection:
		var b []byte
		if item.Literal != nil {
			var err error
			b, err = io.ReadAll(item.Literal)
			if err != nil {
				return err
			}
		}
		if buf.BinarySection == nil {
			buf.BinarySection = make(map[*imap.FetchItemBinarySection][]byte)
		}
		buf.BinarySection[item.Section] = b
	case FetchItemDataFlags:
		buf.Flags = item.Flags
	case FetchItemDataEnvelope:
		buf.Envelope = item.Envelope
	case FetchItemDataInternalDate:
		buf.InternalDate = item.Time
	case FetchItemDataRFC822Size:
		buf.RFC822Size = item.Size
	case FetchItemDataUID:
		buf.UID = item.UID
	case FetchItemDataBodyStructure:
		buf.BodyStructure = item.BodyStructure
	case FetchItemDataBinarySectionSize:
		buf.BinarySectionSize = append(buf.BinarySectionSize, item)
	case FetchItemDataModSeq:
		buf.ModSeq = item.ModSeq
	default:
		panic(fmt.Errorf("不支持的提取项数据 %T", item))
	}
	return nil
}

// handleFetch 处理 FETCH 响应。
// 参数：
// - seqNum: 消息的序列号。
// 返回：
// - 如果解析成功，返回 nil，否则返回错误信息。
func (c *Client) handleFetch(seqNum uint32) error {
	dec := c.dec

	// 创建一个缓冲为 32 的通道，用于存储 FETCH 项目数据
	items := make(chan FetchItemData, 32)
	defer close(items)

	// 创建 FetchMessageData 对象，包含序列号和项目数据
	msg := &FetchMessageData{SeqNum: seqNum, items: items}

	// UID 用于存储消息的唯一标识
	var uid imap.UID
	handled := false

	// handleMsg 是一个内部函数，用于处理 FETCH 响应
	handleMsg := func() {
		if handled {
			return
		}

		// 查找是否有等待处理的命令
		cmd := c.findPendingCmdFunc(func(anyCmd command) bool {
			cmd, ok := anyCmd.(*FetchCommand)
			if !ok {
				return false
			}

			// 根据 UID 或者序列号判断是否处理该消息
			if _, ok := cmd.numSet.(imap.UIDSet); ok {
				return uid != 0 && cmd.recvUID(uid)
			} else {
				return seqNum != 0 && cmd.recvSeqNum(seqNum)
			}
		})

		if cmd != nil {
			// 如果找到等待处理的 FETCH 命令，则将消息发送给该命令
			cmd := cmd.(*FetchCommand)
			cmd.msgs <- msg
		} else if handler := c.options.unilateralDataHandler().Fetch; handler != nil {
			// 如果没有对应的命令，调用非单向数据处理函数
			go handler(msg)
		} else {
			// 如果都没有处理函数，丢弃该消息
			go msg.discard()
		}

		// 标记该消息已处理
		handled = true
	}

	// 在函数结束前处理消息
	defer handleMsg()

	// numAtts 记录当前消息属性的数量
	numAtts := 0

	// 解析 FETCH 属性列表
	return dec.ExpectList(func() error {
		var attName string
		// 解析属性名称
		if !dec.Expect(dec.Func(&attName, isMsgAttNameChar), "消息属性名称") {
			return dec.Err()
		}
		attName = strings.ToUpper(attName)

		var (
			item FetchItemData
			done chan struct{}
		)

		// 根据属性名称处理不同的 FETCH 数据项
		switch attName {
		case "FLAGS": // 处理标志属性
			if !dec.ExpectSP() {
				return dec.Err()
			}
			flags, err := internal.ExpectFlagList(dec)
			if err != nil {
				return err
			}
			item = FetchItemDataFlags{Flags: flags}

		case "ENVELOPE": // 处理信封属性
			if !dec.ExpectSP() {
				return dec.Err()
			}
			envelope, err := readEnvelope(dec, &c.options)
			if err != nil {
				return fmt.Errorf("解析信封时出错: %v", err)
			}
			item = FetchItemDataEnvelope{Envelope: envelope}

		case "INTERNALDATE": // 处理内部日期属性
			if !dec.ExpectSP() {
				return dec.Err()
			}
			t, err := internal.ExpectDateTime(dec)
			if err != nil {
				return err
			}
			item = FetchItemDataInternalDate{Time: t}

		case "RFC822.SIZE": // 处理邮件大小属性
			var size int64
			if !dec.ExpectSP() || !dec.ExpectNumber64(&size) {
				return dec.Err()
			}
			item = FetchItemDataRFC822Size{Size: size}

		case "UID": // 处理 UID 属性
			if !dec.ExpectSP() || !dec.ExpectUID(&uid) {
				return dec.Err()
			}
			item = FetchItemDataUID{UID: uid}

		// 处理 BODY 和 BINARY 属性
		case "BODY", "BINARY":
			if dec.Special('[') {
				var section interface{}
				switch attName {
				case "BODY":
					var err error
					section, err = readSectionSpec(dec)
					if err != nil {
						return fmt.Errorf("解析节信息时出错: %v", err)
					}
				case "BINARY":
					part, dot := readSectionPart(dec)
					if dot {
						return fmt.Errorf("解析二进制节时出错：在点号后预期有数字")
					}
					if !dec.ExpectSpecial(']') {
						return dec.Err()
					}
					section = &imap.FetchItemBinarySection{Part: part}
				}

				if !dec.ExpectSP() {
					return dec.Err()
				}

				// 忽略 literal8 标记
				if attName == "BINARY" {
					dec.Special('~')
				}

				lit, _, ok := dec.ExpectNStringReader()
				if !ok {
					return dec.Err()
				}

				var fetchLit imap.LiteralReader
				if lit != nil {
					done = make(chan struct{})
					fetchLit = &fetchLiteralReader{
						LiteralReader: lit,
						ch:            done,
					}
				}

				switch section := section.(type) {
				case *imap.FetchItemBodySection:
					item = FetchItemDataBodySection{
						Section: section,
						Literal: fetchLit,
					}
				case *imap.FetchItemBinarySection:
					item = FetchItemDataBinarySection{
						Section: section,
						Literal: fetchLit,
					}
				}
				break
			}

			if !dec.Expect(attName == "BODY", "'['") {
				return dec.Err()
			}

			fallthrough

		case "BODYSTRUCTURE": // 处理邮件体结构
			if !dec.ExpectSP() {
				return dec.Err()
			}
			bodyStruct, err := readBody(dec, &c.options)
			if err != nil {
				return err
			}
			item = FetchItemDataBodyStructure{
				BodyStructure: bodyStruct,
				IsExtended:    attName == "BODYSTRUCTURE",
			}

		case "BINARY.SIZE": // 处理二进制大小属性
			if !dec.ExpectSpecial('[') {
				return dec.Err()
			}
			part, dot := readSectionPart(dec)
			if dot {
				return fmt.Errorf("解析二进制节时出错：在点号后预期有数字")
			}

			var size uint32
			if !dec.ExpectSpecial(']') || !dec.ExpectSP() || !dec.ExpectNumber(&size) {
				return dec.Err()
			}
			item = FetchItemDataBinarySectionSize{
				Part: part,
				Size: size,
			}

		case "MODSEQ": // 处理 MODSEQ 属性
			var modSeq uint64
			if !dec.ExpectSP() || !dec.ExpectSpecial('(') || !dec.ExpectModSeq(&modSeq) || !dec.ExpectSpecial(')') {
				return dec.Err()
			}
			item = FetchItemDataModSeq{ModSeq: modSeq}

		default: // 如果属性不支持，返回错误
			return fmt.Errorf("不支持的消息属性名称: %q", attName)
		}

		// 递增属性计数器
		numAtts++
		if numAtts > cap(items) || done != nil {
			handleMsg()
		}

		if done != nil {
			c.setReadTimeout(literalReadTimeout)
		}

		// 将处理完的项发送到通道
		items <- item

		if done != nil {
			<-done
			c.setReadTimeout(respReadTimeout)
		}

		return nil
	})
}

// 判断字符是否是消息属性名称的合法字符
// 参数:
//
//	ch: byte 类型，表示要检查的字符
//
// 返回值:
//
//	返回布尔值，true 表示是合法字符，false 表示非法
func isMsgAttNameChar(ch byte) bool {
	// 检查字符是否不是 '[' 并且是合法的 atom 字符
	return ch != '[' && imapwire.IsAtomChar(ch)
}

// 读取邮件信封（Envelope）信息
// 参数:
//
//	dec: 指向 `imapwire.Decoder` 的指针，表示 IMAP 协议的解码器
//	options: 指向 `Options` 的指针，表示解码选项
//
// 返回值:
//
//	返回 `*imap.Envelope`（邮件信封） 和 错误对象
func readEnvelope(dec *imapwire.Decoder, options *Options) (*imap.Envelope, error) {
	var envelope imap.Envelope

	// 期待一个 '(' 开始
	if !dec.ExpectSpecial('(') {
		return nil, dec.Err() // 如果未能匹配到 '('，返回错误
	}

	// 解析日期和主题
	var date, subject string
	if !dec.ExpectNString(&date) || !dec.ExpectSP() || !dec.ExpectNString(&subject) || !dec.ExpectSP() {
		return nil, dec.Err() // 如果解析失败，返回错误
	}
	// 解析和设置邮件信封中的日期和主题字段
	envelope.Date, _ = netmail.ParseDate(date)
	envelope.Subject, _ = options.decodeText(subject)

	// 解析邮件地址列表
	addrLists := []struct {
		name string
		out  *[]imap.Address
	}{
		{"邮件发件人", &envelope.From},
		{"邮件发送人", &envelope.Sender},
		{"回复地址", &envelope.ReplyTo},
		{"收件人", &envelope.To},
		{"抄送人", &envelope.Cc},
		{"密送人", &envelope.Bcc},
	}
	for _, addrList := range addrLists {
		l, err := readAddressList(dec, options)
		if err != nil {
			return nil, fmt.Errorf("解析 %v 时出错: %v", addrList.name, err) // 错误信息转换为中文
		} else if !dec.ExpectSP() {
			return nil, dec.Err()
		}
		*addrList.out = l
	}

	// 解析 In-Reply-To 和 Message-ID 字段
	var inReplyTo, messageID string
	if !dec.ExpectNString(&inReplyTo) || !dec.ExpectSP() || !dec.ExpectNString(&messageID) {
		return nil, dec.Err()
	}
	envelope.InReplyTo, _ = parseMsgIDList(inReplyTo)
	envelope.MessageID, _ = parseMsgID(messageID)

	// 期待一个 ')' 结束
	if !dec.ExpectSpecial(')') {
		return nil, dec.Err()
	}
	return &envelope, nil
}

// 读取邮件地址列表
// 参数:
//
//	dec: 指向 `imapwire.Decoder` 的指针，表示 IMAP 协议的解码器
//	options: 指向 `Options` 的指针，表示解码选项
//
// 返回值:
//
//	返回 `[]imap.Address`（地址列表）和 错误对象
func readAddressList(dec *imapwire.Decoder, options *Options) ([]imap.Address, error) {
	var l []imap.Address
	// 解析地址列表
	err := dec.ExpectNList(func() error {
		addr, err := readAddress(dec, options)
		if err != nil {
			return err
		}
		l = append(l, *addr)
		return nil
	})
	return l, err
}

// 读取单个邮件地址
// 参数:
//
//	dec: 指向 `imapwire.Decoder` 的指针，表示 IMAP 协议的解码器
//	options: 指向 `Options` 的指针，表示解码选项
//
// 返回值:
//
//	返回 `*imap.Address`（地址）和 错误对象
func readAddress(dec *imapwire.Decoder, options *Options) (*imap.Address, error) {
	var (
		addr     imap.Address
		name     string
		obsRoute string
	)
	// 解析地址各部分
	ok := dec.ExpectSpecial('(') &&
		dec.ExpectNString(&name) && dec.ExpectSP() &&
		dec.ExpectNString(&obsRoute) && dec.ExpectSP() &&
		dec.ExpectNString(&addr.Mailbox) && dec.ExpectSP() &&
		dec.ExpectNString(&addr.Host) && dec.ExpectSpecial(')')
	if !ok {
		return nil, fmt.Errorf("解析地址时出错: %v", dec.Err()) // 错误信息转换为中文
	}
	addr.Name, _ = options.decodeText(name)
	return &addr, nil
}

// parseMsgID 解析消息的 Message-Id 字段
// 参数: s - 输入的消息 ID 字符串
// 返回值: 返回解析后的消息 ID 字符串以及可能的错误
func parseMsgID(s string) (string, error) {
	var h mail.Header
	h.Set("消息-ID", s) // 设置消息的 Message-Id 字段
	return h.MessageID()
}

// parseMsgIDList 解析消息的 In-Reply-To 字段，返回 ID 列表
// 参数: s - 输入的消息 In-Reply-To 字符串
// 返回值: 返回解析后的消息 ID 列表和可能的错误
func parseMsgIDList(s string) ([]string, error) {
	var h mail.Header
	h.Set("回复-ID", s) // 设置消息的 In-Reply-To 字段
	return h.MsgIDList("回复-ID")
}

// readBody 解析消息体结构
// 参数:
// - dec: IMAP 数据解码器，用于读取 IMAP 数据
// - options: Options 配置项，用于设置解析选项
// 返回值: 返回解析后的消息体结构和可能的错误
func readBody(dec *imapwire.Decoder, options *Options) (imap.BodyStructure, error) {
	if !dec.ExpectSpecial('(') {
		return nil, dec.Err() // 如果不是以 '(' 开始，则返回错误
	}

	var (
		mediaType string
		token     string
		bs        imap.BodyStructure
		err       error
	)

	// 解析媒体类型
	if dec.String(&mediaType) {
		token = "单部分消息体类型"
		bs, err = readBodyType1part(dec, mediaType, options)
	} else {
		token = "多部分消息体类型"
		bs, err = readBodyTypeMpart(dec, options)
	}

	if err != nil {
		return nil, fmt.Errorf("在 %v: %v", token, err) // 返回错误信息，包含上下文
	}

	for dec.SP() {
		if !dec.DiscardValue() {
			return nil, dec.Err() // 如果无法跳过值，则返回错误
		}
	}

	if !dec.ExpectSpecial(')') {
		return nil, dec.Err() // 如果不是以 ')' 结束，则返回错误
	}

	return bs, nil // 返回解析后的消息体结构
}

// readBodyType1part 解析单部分消息体结构
// 参数:
// - dec: IMAP 数据解码器
// - typ: 消息体类型字符串
// - options: 解析选项
// 返回值: 返回解析后的单部分消息体结构和可能的错误
func readBodyType1part(dec *imapwire.Decoder, typ string, options *Options) (*imap.BodyStructureSinglePart, error) {
	bs := imap.BodyStructureSinglePart{Type: typ} // 创建单部分消息体结构

	if !dec.ExpectSP() || !dec.ExpectString(&bs.Subtype) || !dec.ExpectSP() {
		return nil, dec.Err() // 如果解析失败，返回错误
	}
	var err error
	bs.Params, err = readBodyFldParam(dec, options) // 解析消息体参数
	if err != nil {
		return nil, err
	}

	var description string
	if !dec.ExpectSP() || !dec.ExpectNString(&bs.ID) || !dec.ExpectSP() || !dec.ExpectNString(&description) || !dec.ExpectSP() || !dec.ExpectNString(&bs.Encoding) || !dec.ExpectSP() || !dec.ExpectBodyFldOctets(&bs.Size) {
		return nil, dec.Err() // 解析消息体的各个字段
	}

	// 默认编码设置为 7BIT，如果为空
	if bs.Encoding == "" {
		bs.Encoding = "7BIT"
	}

	// TODO: 处理错误
	bs.Description, _ = options.decodeText(description) // 解析描述字段

	// 处理 message 和 text 类型的特殊情况
	hasSP := dec.SP()
	if !hasSP {
		return &bs, nil // 如果没有其他字段，返回消息体
	}

	// 处理 message 类型的特殊情况
	if strings.EqualFold(bs.Type, "message") && (strings.EqualFold(bs.Subtype, "rfc822") || strings.EqualFold(bs.Subtype, "global")) {
		var msg imap.BodyStructureMessageRFC822

		msg.Envelope, err = readEnvelope(dec, options) // 读取信封信息
		if err != nil {
			return nil, err
		}

		if !dec.ExpectSP() {
			return nil, dec.Err()
		}

		msg.BodyStructure, err = readBody(dec, options) // 读取内部消息体
		if err != nil {
			return nil, dec.Err()
		}

		if !dec.ExpectSP() || !dec.ExpectNumber64(&msg.NumLines) {
			return nil, dec.Err()
		}

		bs.MessageRFC822 = &msg
		hasSP = false
	} else if strings.EqualFold(bs.Type, "text") {
		var text imap.BodyStructureText

		if !dec.ExpectNumber64(&text.NumLines) {
			return nil, dec.Err()
		}

		bs.Text = &text
		hasSP = false
	}

	// 处理扩展部分
	if !hasSP {
		hasSP = dec.SP()
	}
	if hasSP {
		bs.Extended, err = readBodyExt1part(dec, options)
		if err != nil {
			return nil, fmt.Errorf("在 body-ext-1part 中: %v", err)
		}
	}

	return &bs, nil // 返回单部分消息体结构
}

// readBodyExt1part 解析单部分消息体的扩展字段
// 参数:
// - dec: IMAP 数据解码器
// - options: 解析选项
// 返回值: 返回扩展的单部分消息体结构和可能的错误
func readBodyExt1part(dec *imapwire.Decoder, options *Options) (*imap.BodyStructureSinglePartExt, error) {
	var ext imap.BodyStructureSinglePartExt // 创建扩展结构体

	var md5 string
	if !dec.ExpectNString(&md5) {
		return nil, dec.Err() // 解析 MD5 值
	}

	if !dec.SP() {
		return &ext, nil // 如果没有其他扩展字段，返回扩展结构体
	}

	var err error
	ext.Disposition, err = readBodyFldDsp(dec, options) // 读取 disposition 字段
	if err != nil {
		return nil, fmt.Errorf("在 body-fld-dsp 中: %v", err)
	}

	if !dec.SP() {
		return &ext, nil
	}

	ext.Language, err = readBodyFldLang(dec) // 读取语言字段
	if err != nil {
		return nil, fmt.Errorf("在 body-fld-lang 中: %v", err)
	}

	if !dec.SP() {
		return &ext, nil
	}

	if !dec.ExpectNString(&ext.Location) {
		return nil, dec.Err() // 读取位置字段
	}

	return &ext, nil // 返回解析后的扩展结构体
}

// 读取多部分Body类型
// 参数：
// - dec: IMAP协议的解码器，用于解析IMAP命令和数据
// - options: 选项，用于控制解码过程中某些自定义行为
// 返回：
// - *imap.BodyStructureMultiPart: 解析后的多部分Body结构
// - error: 如果发生错误，则返回错误信息
func readBodyTypeMpart(dec *imapwire.Decoder, options *Options) (*imap.BodyStructureMultiPart, error) {
	var bs imap.BodyStructureMultiPart

	for {
		// 读取单个Body结构
		child, err := readBody(dec, options)
		if err != nil {
			return nil, err
		}
		bs.Children = append(bs.Children, child)

		// 检查是否有子类型
		if dec.SP() && dec.String(&bs.Subtype) {
			break
		}
	}

	if dec.SP() {
		// 读取扩展的Body信息
		var err error
		bs.Extended, err = readBodyExtMpart(dec, options)
		if err != nil {
			return nil, fmt.Errorf("在 body-ext-mpart 解析时出错: %v", err)
		}
	}

	return &bs, nil
}

// 读取扩展的多部分Body类型
// 参数：
// - dec: IMAP协议的解码器
// - options: 选项
// 返回：
// - *imap.BodyStructureMultiPartExt: 解析后的扩展多部分Body结构
// - error: 错误信息
func readBodyExtMpart(dec *imapwire.Decoder, options *Options) (*imap.BodyStructureMultiPartExt, error) {
	var ext imap.BodyStructureMultiPartExt

	var err error
	ext.Params, err = readBodyFldParam(dec, options)
	if err != nil {
		return nil, fmt.Errorf("在 body-fld-param 解析时出错: %v", err)
	}

	if !dec.SP() {
		return &ext, nil
	}

	ext.Disposition, err = readBodyFldDsp(dec, options)
	if err != nil {
		return nil, fmt.Errorf("在 body-fld-dsp 解析时出错: %v", err)
	}

	if !dec.SP() {
		return &ext, nil
	}

	ext.Language, err = readBodyFldLang(dec)
	if err != nil {
		return nil, fmt.Errorf("在 body-fld-lang 解析时出错: %v", err)
	}

	if !dec.SP() {
		return &ext, nil
	}

	if !dec.ExpectNString(&ext.Location) {
		return nil, dec.Err()
	}

	return &ext, nil
}

// 读取Body的Disposition字段
// 参数：
// - dec: IMAP协议的解码器
// - options: 选项
// 返回：
// - *imap.BodyStructureDisposition: 解析后的Disposition字段
// - error: 错误信息
func readBodyFldDsp(dec *imapwire.Decoder, options *Options) (*imap.BodyStructureDisposition, error) {
	if !dec.Special('(') {
		if !dec.ExpectNIL() {
			return nil, dec.Err()
		}
		return nil, nil
	}

	var disp imap.BodyStructureDisposition
	if !dec.ExpectString(&disp.Value) || !dec.ExpectSP() {
		return nil, dec.Err()
	}

	var err error
	disp.Params, err = readBodyFldParam(dec, options)
	if err != nil {
		return nil, err
	}
	if !dec.ExpectSpecial(')') {
		return nil, dec.Err()
	}
	return &disp, nil
}

// 读取Body的参数字段
// 参数：
// - dec: IMAP协议的解码器
// - options: 选项
// 返回：
// - map[string]string: 解析后的参数键值对
// - error: 错误信息
func readBodyFldParam(dec *imapwire.Decoder, options *Options) (map[string]string, error) {
	var (
		params map[string]string
		k      string
	)
	err := dec.ExpectNList(func() error {
		var s string
		if !dec.ExpectString(&s) {
			return dec.Err()
		}

		if k == "" {
			k = s
		} else {
			if params == nil {
				params = make(map[string]string)
			}
			decoded, _ := options.decodeText(s)
			// TODO: 处理错误

			params[strings.ToLower(k)] = decoded
			k = ""
		}

		return nil
	})
	if err != nil {
		return nil, err
	} else if k != "" {
		return nil, fmt.Errorf("在 body-fld-param 解析时出错: 有键但无值")
	}
	return params, nil
}

// 读取Body的语言字段
// 参数：
// - dec: IMAP协议的解码器
// 返回：
// - []string: 解析后的语言列表
// - error: 错误信息
func readBodyFldLang(dec *imapwire.Decoder) ([]string, error) {
	var l []string
	isList, err := dec.List(func() error {
		var s string
		if !dec.ExpectString(&s) {
			return dec.Err()
		}
		l = append(l, s)
		return nil
	})
	if err != nil || isList {
		return l, err
	}

	var s string
	if !dec.ExpectNString(&s) {
		return nil, dec.Err()
	}
	if s != "" {
		return []string{s}, nil
	} else {
		return nil, nil
	}
}

// 读取部分Body的节段说明
// 参数：
// - dec: IMAP协议的解码器
// 返回：
// - *imap.FetchItemBodySection: 解析后的节段说明
// - error: 错误信息
func readSectionSpec(dec *imapwire.Decoder) (*imap.FetchItemBodySection, error) {
	var section imap.FetchItemBodySection

	var dot bool
	section.Part, dot = readSectionPart(dec)
	if dot || len(section.Part) == 0 {
		var specifier string
		if dot {
			if !dec.ExpectAtom(&specifier) {
				return nil, dec.Err()
			}
		} else {
			dec.Atom(&specifier)
		}
		specifier = strings.ToUpper(specifier)
		section.Specifier = imap.PartSpecifier(specifier)

		if specifier == "HEADER.FIELDS" || specifier == "HEADER.FIELDS.NOT" {
			if !dec.ExpectSP() {
				return nil, dec.Err()
			}
			var err error
			headerList, err := readHeaderList(dec)
			if err != nil {
				return nil, err
			}
			section.Specifier = imap.PartSpecifierHeader
			if specifier == "HEADER.FIELDS" {
				section.HeaderFields = headerList
			} else {
				section.HeaderFieldsNot = headerList
			}
		}
	}

	if !dec.ExpectSpecial(']') {
		return nil, dec.Err()
	}

	offset, err := readPartialOffset(dec)
	if err != nil {
		return nil, err
	}
	if offset != nil {
		section.Partial = &imap.SectionPartial{Offset: int64(*offset)}
	}

	return &section, nil
}

// 读取节段偏移量
// 参数：
// - dec: IMAP协议的解码器
// 返回：
// - *uint32: 解析后的偏移量
// - error: 错误信息
func readPartialOffset(dec *imapwire.Decoder) (*uint32, error) {
	if !dec.Special('<') {
		return nil, nil
	}
	var offset uint32
	if !dec.ExpectNumber(&offset) || !dec.ExpectSpecial('>') {
		return nil, dec.Err()
	}
	return &offset, nil
}

// 读取Header列表
// 参数：
// - dec: IMAP协议的解码器
// 返回：
// - []string: 解析后的Header列表
// - error: 错误信息
func readHeaderList(dec *imapwire.Decoder) ([]string, error) {
	var l []string
	err := dec.ExpectList(func() error {
		var s string
		if !dec.ExpectAString(&s) {
			return dec.Err()
		}
		l = append(l, s)
		return nil
	})
	return l, err
}

// 读取节段部分
// 参数：
// - dec: IMAP协议的解码器
// 返回：
// - part []int: 解析后的节段部分
// - dot bool: 是否有点号分隔符
func readSectionPart(dec *imapwire.Decoder) (part []int, dot bool) {
	for {
		dot = len(part) > 0
		if dot && !dec.Special('.') {
			return part, false
		}

		var num uint32
		if !dec.Number(&num) {
			return part, dot
		}
		part = append(part, int(num))
	}
}

// fetchLiteralReader结构体，用于读取IMAP中的字面量数据
// 字段：
// - LiteralReader: 基础的字面量读取器
// - ch: 通知通道，在字面量读取结束时关闭
type fetchLiteralReader struct {
	*imapwire.LiteralReader
	ch chan<- struct{}
}

// 读取字面量数据
// 参数：
// - b []byte: 数据缓冲区
// 返回：
// - int: 读取的字节数
// - error: 如果有错误则返回错误信息
func (lit *fetchLiteralReader) Read(b []byte) (int, error) {
	n, err := lit.LiteralReader.Read(b)
	if err == io.EOF && lit.ch != nil {
		close(lit.ch)
		lit.ch = nil
	}
	return n, err
}
