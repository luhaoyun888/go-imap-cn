package imapserver

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleNamespace 处理命名空间请求。
// 参数：
//
//	dec - 解码器，用于解析请求数据
//
// 返回：错误信息，如果有的话
func (c *Conn) handleNamespace(dec *imapwire.Decoder) error {
	if !dec.ExpectCRLF() {
		return dec.Err() // 检查是否以 CRLF 结束
	}

	// 检查连接状态是否为已认证状态
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err // 返回状态检查错误
	}

	// 检查当前会话是否支持命名空间操作
	session, ok := c.session.(SessionNamespace)
	if !ok {
		return newClientBugError("命名空间不被支持") // 返回客户端错误信息
	}

	// 获取命名空间数据
	data, err := session.Namespace()
	if err != nil {
		return err // 返回获取命名空间数据的错误
	}

	// 创建响应编码器
	enc := newResponseEncoder(c)
	defer enc.end()                            // 确保结束编码器
	enc.Atom("*").SP().Atom("NAMESPACE").SP()  // 编码响应头
	writeNamespace(enc.Encoder, data.Personal) // 编码个人命名空间
	enc.SP()
	writeNamespace(enc.Encoder, data.Other) // 编码其他命名空间
	enc.SP()
	writeNamespace(enc.Encoder, data.Shared) // 编码共享命名空间
	return enc.CRLF()                        // 返回结束标记
}

// writeNamespace 编码命名空间描述符列表。
// 参数：
//
//	enc - 编码器，用于写入命名空间数据
//	l - 命名空间描述符列表
func writeNamespace(enc *imapwire.Encoder, l []imap.NamespaceDescriptor) {
	if l == nil {
		enc.NIL() // 如果列表为空，写入 NIL
		return
	}

	enc.List(len(l), func(i int) {
		descr := l[i]
		enc.Special('(').String(descr.Prefix).SP() // 编码命名空间前缀
		if descr.Delim == 0 {
			enc.NIL() // 如果分隔符为 0，写入 NIL
		} else {
			enc.Quoted(string(descr.Delim)) // 编码分隔符
		}
		enc.Special(')') // 结束命名空间描述符
	})
}
