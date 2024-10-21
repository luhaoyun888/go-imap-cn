package imapserver

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleStore 处理 STORE 命令。
func (c *Conn) handleStore(dec *imapwire.Decoder, numKind NumKind) error {
	var (
		numSet imap.NumSet // 存储的消息集合
		item   string      // 要修改的项目
	)

	// 检查命令格式，确保包括数字集合和项目名称
	if !dec.ExpectSP() || !dec.ExpectNumSet(numKind.wire(), &numSet) || !dec.ExpectSP() || !dec.ExpectAtom(&item) || !dec.ExpectSP() {
		return dec.Err() // 返回解码错误
	}

	var flags []imap.Flag // 存储标志
	isList, err := dec.List(func() error {
		flag, err := internal.ExpectFlag(dec) // 读取标志
		if err != nil {
			return err // 返回读取错误
		}
		flags = append(flags, flag) // 将标志添加到列表
		return nil
	})
	if err != nil {
		return err // 返回解析错误
	} else if !isList {
		for {
			flag, err := internal.ExpectFlag(dec) // 读取标志
			if err != nil {
				return err // 返回读取错误
			}
			flags = append(flags, flag) // 将标志添加到列表

			if !dec.SP() { // 检查是否还有其他标志
				break
			}
		}
	}
	if !dec.ExpectCRLF() { // 检查命令是否以 CRLF 结束
		return dec.Err() // 返回解码错误
	}

	item = strings.ToUpper(item) // 将项目名称转为大写
	silent := strings.HasSuffix(item, ".SILENT") // 检查是否为 SILENT 标志
	item = strings.TrimSuffix(item, ".SILENT") // 移除 SILENT 后缀

	var op imap.StoreFlagsOp // 操作类型
	switch {
	case strings.HasPrefix(item, "+"):
		op = imap.StoreFlagsAdd // 添加标志
		item = strings.TrimPrefix(item, "+") // 移除前缀
	case strings.HasPrefix(item, "-"):
		op = imap.StoreFlagsDel // 删除标志
		item = strings.TrimPrefix(item, "-") // 移除前缀
	default:
		op = imap.StoreFlagsSet // 设置标志
	}

	if item != "FLAGS" { // 仅支持 FLAGS 项目
		return newClientBugError("STORE 只能更改 FLAGS") // 返回错误
	}

	if err := c.checkState(imap.ConnStateSelected); err != nil { // 检查连接状态是否为已选择
		return err
	}

	w := &FetchWriter{conn: c} // 创建 FetchWriter
	options := imap.StoreOptions{} // 创建存储选项
	return c.session.Store(w, numSet, &imap.StoreFlags{
		Op:     op,
		Silent: silent,
		Flags:  flags,
	}, &options) // 调用会话的 Store 方法
}
