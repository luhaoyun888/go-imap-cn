package imapclient

import (
	"fmt"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// Store 发送一个 STORE 命令。
//
// 除非 StoreFlags.Silent 被设置，服务器将返回更新后的值。
//
// nil 的 options 指针等同于零选项值。
func (c *Client) Store(numSet imap.NumSet, store *imap.StoreFlags, options *imap.StoreOptions) *FetchCommand {
	cmd := &FetchCommand{
		numSet: numSet,
		msgs:   make(chan *FetchMessageData, 128), // 创建消息数据通道
	}
	enc := c.beginCommand(uidCmdName("STORE", imapwire.NumSetKind(numSet)), cmd)
	enc.SP().NumSet(numSet).SP() // 添加序列集

	// 如果选项不为 nil 且 UnchangedSince 不为 0，添加 UNCHANGEDSINCE 条件
	if options != nil && options.UnchangedSince != 0 {
		enc.Special('(').Atom("UNCHANGEDSINCE").SP().ModSeq(options.UnchangedSince).Special(')').SP()
	}

	// 根据操作类型设置标志
	switch store.Op {
	case imap.StoreFlagsSet:
		// 无需操作
	case imap.StoreFlagsAdd:
		enc.Special('+') // 添加标志
	case imap.StoreFlagsDel:
		enc.Special('-') // 删除标志
	default:
		panic(fmt.Errorf("imapclient: 未知的存储标志操作: %v", store.Op)) // 处理未知操作
	}

	enc.Atom("FLAGS") // 添加 FLAGS 关键字
	if store.Silent {
		enc.Atom(".SILENT") // 如果 Silent 被设置，添加 .SILENT
	}

	// 添加标志列表
	enc.SP().List(len(store.Flags), func(i int) {
		enc.Flag(store.Flags[i])
	})

	enc.end()  // 结束编码
	return cmd // 返回命令
}
