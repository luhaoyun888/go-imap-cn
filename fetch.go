package imap

import (
	"fmt"
	"strings"
	"time"
)

// FetchOptions 包含 FETCH 命令的选项。
type FetchOptions struct {
	// 要获取的字段
	BodyStructure     *FetchItemBodyStructure       // 消息的体结构
	Envelope          bool                          // 是否获取信封信息
	Flags             bool                          // 是否获取标志
	InternalDate      bool                          // 是否获取内部日期
	RFC822Size        bool                          // 是否获取 RFC822 大小
	UID               bool                          // 是否获取 UID
	BodySection       []*FetchItemBodySection       // 体部分
	BinarySection     []*FetchItemBinarySection     // 二进制部分（要求支持 IMAP4rev2 或 BINARY）
	BinarySectionSize []*FetchItemBinarySectionSize // 二进制部分大小（要求支持 IMAP4rev2 或 BINARY）
	ModSeq            bool                          // 是否获取修改序列（要求支持 CONDSTORE）

	ChangedSince uint64 // 从某个修改时间点后获取
}

// FetchItemBodyStructure 包含用于体结构获取的 FETCH 选项。
type FetchItemBodyStructure struct {
	Extended bool // 是否获取扩展信息
}

// PartSpecifier 描述要获取的部分的头、体或两者。
type PartSpecifier string

const (
	PartSpecifierNone   PartSpecifier = ""       // 不获取任何部分
	PartSpecifierHeader PartSpecifier = "HEADER" // 获取头部
	PartSpecifierMIME   PartSpecifier = "MIME"   // 获取 MIME 部分
	PartSpecifierText   PartSpecifier = "TEXT"   // 获取文本部分
)

// SectionPartial 描述获取消息有效载荷时的字节范围。
type SectionPartial struct {
	Offset, Size int64 // 偏移量和大小
}

// FetchItemBodySection 是一个 FETCH BODY[] 数据项。
//
// 要获取消息的完整体，使用零的 FetchItemBodySection：
// imap.FetchItemBodySection{}
//
// 要仅获取特定部分，使用 Part 字段：
// imap.FetchItemBodySection{Part: []int{1, 2, 3}}
//
// 要仅获取消息的头部，使用 Specifier 字段：
// imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader}
type FetchItemBodySection struct {
	Specifier       PartSpecifier   // 指定获取的部分类型
	Part            []int           // 指定部分的索引
	HeaderFields    []string        // 指定要获取的头部字段
	HeaderFieldsNot []string        // 指定不获取的头部字段
	Partial         *SectionPartial // 指定部分内容的偏移和大小
	Peek            bool            // 是否使用 Peek 模式
}

// FetchItemBinarySection 是一个 FETCH BINARY[] 数据项。
type FetchItemBinarySection struct {
	Part    []int           // 指定部分的索引
	Partial *SectionPartial // 指定部分内容的偏移和大小
	Peek    bool            // 是否使用 Peek 模式
}

// FetchItemBinarySectionSize 是一个 FETCH BINARY.SIZE[] 数据项。
type FetchItemBinarySectionSize struct {
	Part []int // 指定部分的索引
}

// Envelope 是消息的信封结构。
//
// 主题和地址采用 UTF-8 格式（即非编码形式）。In-Reply-To 和 Message-ID 的值包含没有尖括号的消息标识符。
type Envelope struct {
	Date      time.Time // 消息日期
	Subject   string    // 主题
	From      []Address // 发件人地址
	Sender    []Address // 发送者地址
	ReplyTo   []Address // 回复地址
	To        []Address // 收件人地址
	Cc        []Address // 抄送地址
	Bcc       []Address // 密送地址
	InReplyTo []string  // 引用的消息 ID
	MessageID string    // 消息 ID
}

// Address 表示消息的发送者或接收者。
type Address struct {
	Name    string // 名称
	Mailbox string // 邮箱名
	Host    string // 主机
}

// Addr 返回邮件地址，格式为 "foo@example.org"。
//
// 如果地址是组的开始或结束，则返回空字符串。
func (addr *Address) Addr() string {
	if addr.Mailbox == "" || addr.Host == "" {
		return ""
	}
	return addr.Mailbox + "@" + addr.Host
}

// IsGroupStart 返回如果该地址是组的开始标记则为真。
//
// 在这种情况下，Mailbox 包含组名短语。
func (addr *Address) IsGroupStart() bool {
	return addr.Host == "" && addr.Mailbox != ""
}

// IsGroupEnd 返回如果该地址是组的结束标记则为真。
func (addr *Address) IsGroupEnd() bool {
	return addr.Host == "" && addr.Mailbox == ""
}

// BodyStructure 描述消息的体结构。
//
// BodyStructure 值可以是 *BodyStructureSinglePart 或 *BodyStructureMultiPart。
type BodyStructure interface {
	// MediaType 返回该体结构的 MIME 类型，例如 "text/plain"。
	MediaType() string
	// Walk 遍历体结构树，对每个部分调用 f，
	// 包括 bs 本身。部分按 DFS 前序访问。
	Walk(f BodyStructureWalkFunc)
	// Disposition 返回体结构的处置方式（如果可用）。
	Disposition() *BodyStructureDisposition

	bodyStructure()
}

// BodyStructureSinglePart 是具有单个部分的体结构。
type BodyStructureSinglePart struct {
	Type, Subtype string            // MIME 类型和子类型
	Params        map[string]string // 参数
	ID            string            // ID
	Description   string            // 描述
	Encoding      string            // 编码
	Size          uint32            // 大小

	MessageRFC822 *BodyStructureMessageRFC822 // 仅适用于 "message/rfc822"
	Text          *BodyStructureText          // 仅适用于 "text/*"
	Extended      *BodyStructureSinglePartExt // 扩展数据
}

func (bs *BodyStructureSinglePart) MediaType() string {
	return strings.ToLower(bs.Type) + "/" + strings.ToLower(bs.Subtype)
}

func (bs *BodyStructureSinglePart) Walk(f BodyStructureWalkFunc) {
	f([]int{1}, bs)
}

func (bs *BodyStructureSinglePart) Disposition() *BodyStructureDisposition {
	if bs.Extended == nil {
		return nil
	}
	return bs.Extended.Disposition
}

// Filename 解码体结构的文件名（如果有的话）。
func (bs *BodyStructureSinglePart) Filename() string {
	var filename string
	if bs.Extended != nil && bs.Extended.Disposition != nil {
		filename = bs.Extended.Disposition.Params["filename"]
	}
	if filename == "" {
		// 注意：在 Content-Type 中使用 "name" 是不建议的
		filename = bs.Params["name"]
	}
	return filename
}

func (*BodyStructureSinglePart) bodyStructure() {}

// BodyStructureMessageRFC822 包含针对 BodyStructureSinglePart 的 RFC 822 部分的元数据。
type BodyStructureMessageRFC822 struct {
	Envelope      *Envelope     // 消息信封
	BodyStructure BodyStructure // 消息体结构
	NumLines      int64         // 行数
}

// BodyStructureText 包含针对 BodyStructureSinglePart 的文本部分的元数据。
type BodyStructureText struct {
	NumLines int64 // 行数
}

// BodyStructureSinglePartExt 包含针对 BodyStructureSinglePart 的扩展体结构数据。
type BodyStructureSinglePartExt struct {
	Disposition *BodyStructureDisposition // 处置方式
	Language    []string                  // 语言
	Location    string                    // 位置
}

// BodyStructureMultiPart 是具有多个部分的体结构。
type BodyStructureMultiPart struct {
	Children []BodyStructure // 子部分
	Subtype  string          // 子类型

	Extended *BodyStructureMultiPartExt // 扩展数据
}

func (bs *BodyStructureMultiPart) MediaType() string {
	return "multipart/" + strings.ToLower(bs.Subtype)
}

func (bs *BodyStructureMultiPart) Walk(f BodyStructureWalkFunc) {
	bs.walk(f, nil)
}

func (bs *BodyStructureMultiPart) walk(f BodyStructureWalkFunc, path []int) {
	if !f(path, bs) {
		return
	}

	pathBuf := make([]int, len(path))
	copy(pathBuf, path)
	for i, part := range bs.Children {
		num := i + 1
		partPath := append(pathBuf, num)

		switch part := part.(type) {
		case *BodyStructureSinglePart:
			f(partPath, part)
		case *BodyStructureMultiPart:
			part.walk(f, partPath)
		default:
			panic(fmt.Errorf("unsupported body structure type %T", part))
		}
	}
}

func (bs *BodyStructureMultiPart) Disposition() *BodyStructureDisposition {
	if bs.Extended == nil {
		return nil
	}
	return bs.Extended.Disposition
}

func (*BodyStructureMultiPart) bodyStructure() {}

// BodyStructureMultiPartExt 包含针对 BodyStructureMultiPart 的扩展体结构数据。
type BodyStructureMultiPartExt struct {
	Params      map[string]string         // 参数
	Disposition *BodyStructureDisposition // 处置方式
	Language    []string                  // 语言
	Location    string                    // 位置
}

// BodyStructureDisposition 描述部分的内容处置（在 Content-Disposition 头字段中指定）。
type BodyStructureDisposition struct {
	Value  string            // 处置方式
	Params map[string]string // 参数
}

// BodyStructureWalkFunc 是一个函数，用于访问 BodyStructure.Walk 遍历的每个体结构。
//
// path 参数包含 IMAP 部分路径。
//
// 函数应返回 true 以访问所有部分的子项，或 false 以跳过它们。
type BodyStructureWalkFunc func(path []int, part BodyStructure) (walkChildren bool)
