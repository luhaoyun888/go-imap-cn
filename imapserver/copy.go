package imapserver

import (
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// handleCopy 处理 COPY 命令，执行将邮件从源邮箱复制到目标邮箱的操作。
// 参数：
//
//	tag: 客户端请求的标识符，用于标识此命令的响应。
//	dec: 用于解码请求的 imapwire 解码器。
//	numKind: 表示邮件编号的类型（可以是普通编号或 UID）。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) handleCopy(tag string, dec *imapwire.Decoder, numKind NumKind) error {
	numSet, dest, err := readCopy(numKind, dec)
	if err != nil {
		return err
	}
	if err := c.checkState(imap.ConnStateSelected); err != nil {
		return err
	}
	data, err := c.session.Copy(numSet, dest)
	if err != nil {
		return err
	}

	cmdName := "COPY"
	if numKind == NumKindUID {
		cmdName = "UID COPY" // 如果是 UID 拷贝，修改命令名称
	}
	if err := c.poll(cmdName); err != nil {
		return err
	}

	return c.writeCopyOK(tag, data) // 写入成功响应
}

// writeCopyOK 写入成功的 COPY 响应。
// 参数：
//
//	tag: 客户端请求的标识符，用于标识此命令的响应。
//	data: 包含复制操作结果的 imap.CopyData 结构体。
//
// 返回值：
//
//	返回 nil 表示成功，其他返回值表示错误信息。
func (c *Conn) writeCopyOK(tag string, data *imap.CopyData) error {
	enc := newResponseEncoder(c) // 创建一个新的响应编码器
	defer enc.end()              // 确保在函数结束时结束编码

	if tag == "" {
		tag = "*" // 如果没有提供 tag，使用通配符标识符
	}

	// 编码 OK 响应
	enc.Atom(tag).SP().Atom("OK").SP()
	if data != nil {
		enc.Special('[')
		enc.Atom("COPYUID").SP().Number(data.UIDValidity).SP().NumSet(data.SourceUIDs).SP().NumSet(data.DestUIDs)
		enc.Special(']').SP()
	}
	enc.Text("COPY completed") // 添加响应消息
	return enc.CRLF()          // 返回编码后的响应
}

// readCopy 读取 COPY 命令中的参数，包括邮件编号集和目标邮箱。
// 参数：
//
//	numKind: 表示邮件编号的类型（可以是普通编号或 UID）。
//	dec: 用于解码请求的 imapwire 解码器。
//
// 返回值：
//
//	返回邮件编号集（numSet）、目标邮箱名（dest），以及可能发生的错误（err）。
func readCopy(numKind NumKind, dec *imapwire.Decoder) (numSet imap.NumSet, dest string, err error) {
	// 检查解码器是否能够正确解析请求
	if !dec.ExpectSP() || !dec.ExpectNumSet(numKind.wire(), &numSet) || !dec.ExpectSP() || !dec.ExpectMailbox(&dest) || !dec.ExpectCRLF() {
		return nil, "", dec.Err() // 返回错误信息
	}
	return numSet, dest, nil // 返回解析后的邮件编号集和目标邮箱
}
