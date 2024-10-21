// Package imap 实现 IMAP4rev2。
//
// IMAP4rev2 在 RFC 9051 中定义。
//
// 本包包含客户端和服务器通用的类型和函数。请参阅 imapclient 和 imapserver 子包。
package imap

import (
	"fmt"
	"io"
)

// ConnState 描述连接状态。
//
// 请参见 RFC 9051 第 3 节。
type ConnState int

const (
	ConnStateNone             ConnState = iota // 无状态
	ConnStateNotAuthenticated                  // 未认证
	ConnStateAuthenticated                     // 已认证
	ConnStateSelected                          // 已选择
	ConnStateLogout                            // 登出
)

// String 实现 fmt.Stringer 接口。
func (state ConnState) String() string {
	switch state {
	case ConnStateNone:
		return "none" // 无状态
	case ConnStateNotAuthenticated:
		return "not authenticated" // 未认证
	case ConnStateAuthenticated:
		return "authenticated" // 已认证
	case ConnStateSelected:
		return "selected" // 已选择
	case ConnStateLogout:
		return "logout" // 登出
	default:
		panic(fmt.Errorf("imap: unknown connection state %v", int(state))) // 未知状态
	}
}

// MailboxAttr 是邮箱属性。
//
// 邮箱属性在 RFC 9051 第 7.3.1 节中定义。
type MailboxAttr string

const (
	// 基础属性
	MailboxAttrNonExistent   MailboxAttr = "\\NonExistent"   // 不存在
	MailboxAttrNoInferiors   MailboxAttr = "\\Noinferiors"   // 无下级
	MailboxAttrNoSelect      MailboxAttr = "\\Noselect"      // 不可选择
	MailboxAttrHasChildren   MailboxAttr = "\\HasChildren"   // 有子项
	MailboxAttrHasNoChildren MailboxAttr = "\\HasNoChildren" // 无子项
	MailboxAttrMarked        MailboxAttr = "\\Marked"        // 已标记
	MailboxAttrUnmarked      MailboxAttr = "\\Unmarked"      // 未标记
	MailboxAttrSubscribed    MailboxAttr = "\\Subscribed"    // 已订阅
	MailboxAttrRemote        MailboxAttr = "\\Remote"        // 远程

	// 角色（即 "特殊用途"）属性
	MailboxAttrAll       MailboxAttr = "\\All"       // 全部
	MailboxAttrArchive   MailboxAttr = "\\Archive"   // 档案
	MailboxAttrDrafts    MailboxAttr = "\\Drafts"    // 草稿
	MailboxAttrFlagged   MailboxAttr = "\\Flagged"   // 标记
	MailboxAttrJunk      MailboxAttr = "\\Junk"      // 垃圾
	MailboxAttrSent      MailboxAttr = "\\Sent"      // 已发送
	MailboxAttrTrash     MailboxAttr = "\\Trash"     // 垃圾箱
	MailboxAttrImportant MailboxAttr = "\\Important" // 重要（RFC 8457）
)

// Flag 是消息标志。
//
// 消息标志在 RFC 9051 第 2.3.2 节中定义。
type Flag string

const (
	// 系统标志
	FlagSeen     Flag = "\\Seen"     // 已读
	FlagAnswered Flag = "\\Answered" // 已回复
	FlagFlagged  Flag = "\\Flagged"  // 已标记
	FlagDeleted  Flag = "\\Deleted"  // 已删除
	FlagDraft    Flag = "\\Draft"    // 草稿

	// 常用标志
	FlagForwarded Flag = "$Forwarded" // 已转发
	FlagMDNSent   Flag = "$MDNSent"   // 消息处理通知已发送
	FlagJunk      Flag = "$Junk"      // 垃圾
	FlagNotJunk   Flag = "$NotJunk"   // 非垃圾
	FlagPhishing  Flag = "$Phishing"  // 钓鱼
	FlagImportant Flag = "$Important" // 重要（RFC 8457）

	// 永久标志
	FlagWildcard Flag = "\\*" // 通配符
)

// LiteralReader 是 IMAP 字面量的读取器。
type LiteralReader interface {
	io.Reader    // 实现 io.Reader 接口
	Size() int64 // 返回字面量的大小
}

// UID 是消息的唯一标识符。
type UID uint32
