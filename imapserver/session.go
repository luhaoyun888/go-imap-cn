// 包 imapserver 实现了一个 IMAP 服务器。
package imapserver

import (
	"fmt"

	"github.com/emersion/go-sasl"
	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/internal/imapwire"
)

// errAuthFailed 是一个 IMAP 错误，表示认证失败。
var errAuthFailed = &imap.Error{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeAuthenticationFailed,
	Text: "认证失败",
}

// ErrAuthFailed 在 Session.Login 认证失败时返回。
var ErrAuthFailed = errAuthFailed

// GreetingData 是与 IMAP 问候相关的数据。
type GreetingData struct {
	PreAuth bool // 是否预先认证
}

// NumKind 描述一个数字应如何被解释：可以是序列号或 UID。
type NumKind int

const (
	NumKindSeq = NumKind(imapwire.NumKindSeq) // 序列号类型
	NumKindUID = NumKind(imapwire.NumKindUID) // UID 类型
)

// String 实现 fmt.Stringer 接口。
func (kind NumKind) String() string {
	switch kind {
	case NumKindSeq:
		return "seq" // 返回字符串 "seq"
	case NumKindUID:
		return "uid" // 返回字符串 "uid"
	default:
		panic(fmt.Errorf("imapserver: 未知的 NumKind %d", kind)) // 抛出未知类型错误
	}
}

// wire 返回与 NumKind 相关的 wire 类型。
func (kind NumKind) wire() imapwire.NumKind {
	return imapwire.NumKind(kind)
}

// Session 是一个 IMAP 会话接口。
type Session interface {
	Close() error // 关闭会话

	// 未认证状态
	Login(username, password string) error // 登录方法

	// 认证状态
	Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error)                       // 选择邮箱
	Create(mailbox string, options *imap.CreateOptions) error                                           // 创建邮箱
	Delete(mailbox string) error                                                                        // 删除邮箱
	Rename(mailbox, newName string) error                                                               // 重命名邮箱
	Subscribe(mailbox string) error                                                                     // 订阅邮箱
	Unsubscribe(mailbox string) error                                                                   // 取消订阅邮箱
	List(w *ListWriter, ref string, patterns []string, options *imap.ListOptions) error                 // 列出邮箱
	Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error)                       // 获取邮箱状态
	Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) // 追加邮件到邮箱
	Poll(w *UpdateWriter, allowExpunge bool) error                                                      // 获取更新
	Idle(w *UpdateWriter, stop <-chan struct{}) error                                                   // 空闲状态

	// 选择状态
	Unselect() error                                                                                           // 取消选择
	Expunge(w *ExpungeWriter, uids *imap.UIDSet) error                                                         // 清除邮件
	Search(kind NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) // 搜索邮件
	Fetch(w *FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error                                // 获取邮件
	Store(w *FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error        // 存储邮件
	Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error)                                              // 复制邮件
}

// SessionNamespace 是一个支持 NAMESPACE 的 IMAP 会话。
type SessionNamespace interface {
	Session

	// 认证状态
	Namespace() (*imap.NamespaceData, error) // 获取命名空间
}

// SessionMove 是一个支持 MOVE 的 IMAP 会话。
type SessionMove interface {
	Session

	// 选择状态
	Move(w *MoveWriter, numSet imap.NumSet, dest string) error // 移动邮件
}

// SessionIMAP4rev2 是一个支持 IMAP4rev2 的 IMAP 会话。
type SessionIMAP4rev2 interface {
	Session
	SessionNamespace
	SessionMove
}

// SessionSASL 是一个支持其自己 SASL 认证机制的 IMAP 会话。
type SessionSASL interface {
	Session
	AuthenticateMechanisms() []string              // 获取支持的认证机制
	Authenticate(mech string) (sasl.Server, error) // 执行认证
}

// SessionUnauthenticate 是一个支持 UNAUTHENTICATE 的 IMAP 会话。
type SessionUnauthenticate interface {
	Session

	// 认证状态
	Unauthenticate() error // 执行未认证
}
