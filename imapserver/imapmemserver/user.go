package imapmemserver

import (
	"crypto/subtle"
	"sort"
	"strings"
	"sync"

	"github.com/luhaoyun888/go-imap-cn"
	"github.com/luhaoyun888/go-imap-cn/imapserver"
)

const mailboxDelim rune = '/' // 邮箱分隔符

// User 结构体表示一个用户，包含用户的基本信息和邮箱。
type User struct {
	username, password string // 用户名和密码

	mutex           sync.Mutex          // 互斥锁，保护并发访问
	mailboxes       map[string]*Mailbox // 用户的邮箱映射
	prevUidValidity uint32              // 上一个 UID 有效性
}

// NewUser 创建一个新的用户实例。
// 参数：
//   - username: 用户名。
//   - password: 密码。
//
// 返回一个 User 结构体指针。
func NewUser(username, password string) *User {
	return &User{
		username:  username,
		password:  password,
		mailboxes: make(map[string]*Mailbox), // 初始化邮箱映射
	}
}

// Login 方法用于用户登录。
// 参数：
//   - username: 输入的用户名。
//   - password: 输入的密码。
//
// 返回：
//   - 如果登录失败，返回 imapserver.ErrAuthFailed；否则返回 nil。
func (u *User) Login(username, password string) error {
	if username != u.username { // 检查用户名
		return imapserver.ErrAuthFailed
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(u.password)) != 1 { // 安全比较密码
		return imapserver.ErrAuthFailed
	}
	return nil
}

// mailboxLocked 方法在锁定状态下返回指定名称的邮箱。
// 参数：
//   - name: 邮箱名称。
//
// 返回：
//   - 如果邮箱存在，返回对应的 Mailbox；否则返回一个包含错误信息的 imap.Error。
func (u *User) mailboxLocked(name string) (*Mailbox, error) {
	mbox := u.mailboxes[name] // 获取指定名称的邮箱
	if mbox == nil {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeNonExistent, // 邮箱不存在错误代码
			Text: "找不到该邮箱",
		}
	}
	return mbox, nil
}

// mailbox 方法返回指定名称的邮箱，并锁定以确保线程安全。
// 参数：
//   - name: 邮箱名称。
//
// 返回：
//   - 如果邮箱存在，返回对应的 Mailbox；否则返回错误信息。
func (u *User) mailbox(name string) (*Mailbox, error) {
	u.mutex.Lock()               // 锁定
	defer u.mutex.Unlock()       // 解锁
	return u.mailboxLocked(name) // 调用 mailboxLocked 方法
}

// Status 方法返回指定邮箱的状态信息。
// 参数：
//   - name: 邮箱名称。
//   - options: 状态选项。
//
// 返回：
//   - 返回邮箱的状态数据；如果发生错误，返回 nil 和错误信息。
func (u *User) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	mbox, err := u.mailbox(name) // 获取邮箱
	if err != nil {
		return nil, err // 返回错误
	}
	return mbox.StatusData(options), nil // 返回状态数据
}

// List 方法列出用户的邮箱。
// 参数：
//   - w: ListWriter，用于写入结果。
//   - ref: 引用字符串，用于匹配邮箱。
//   - patterns: 匹配模式数组。
//   - options: 列表选项。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	u.mutex.Lock()         // 锁定
	defer u.mutex.Unlock() // 解锁

	// TODO: 如果ref不存在，返回失败

	if len(patterns) == 0 { // 如果没有模式
		return w.WriteList(&imap.ListData{
			Attrs: []imap.MailboxAttr{imap.MailboxAttrNoSelect}, // 标记为不可选择
			Delim: mailboxDelim,                                 // 使用邮箱分隔符
		})
	}

	var l []imap.ListData                 // 存储匹配的邮箱数据
	for name, mbox := range u.mailboxes { // 遍历用户的邮箱
		match := false
		for _, pattern := range patterns { // 对每个模式进行匹配
			match = imapserver.MatchList(name, mailboxDelim, ref, pattern)
			if match {
				break
			}
		}
		if !match {
			continue // 如果没有匹配，跳过
		}

		data := mbox.list(options) // 获取邮箱列表数据
		if data != nil {
			l = append(l, *data) // 添加到结果列表
		}
	}

	// 排序邮箱
	sort.Slice(l, func(i, j int) bool {
		return l[i].Mailbox < l[j].Mailbox
	})

	for _, data := range l { // 写入结果
		if err := w.WriteList(&data); err != nil {
			return err // 返回错误
		}
	}

	return nil // 返回 nil 表示成功
}

// Append 方法向指定邮箱追加邮件。
// 参数：
//   - mailbox: 邮箱名称。
//   - r: 邮件内容的 LiteralReader。
//   - options: 附加选项。
//
// 返回：
//   - 返回附加结果和错误信息（如果有）。
func (u *User) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	mbox, err := u.mailbox(mailbox) // 获取邮箱
	if err != nil {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTryCreate, // 邮箱不存在，提示尝试创建
			Text: "找不到该邮箱",
		}
	}
	return mbox.appendLiteral(r, options) // 追加邮件
}

// Create 方法创建一个新的邮箱。
// 参数：
//   - name: 新邮箱名称。
//   - options: 创建选项。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) Create(name string, options *imap.CreateOptions) error {
	u.mutex.Lock()         // 锁定
	defer u.mutex.Unlock() // 解锁

	name = strings.TrimRight(name, string(mailboxDelim)) // 去掉尾部的分隔符

	if u.mailboxes[name] != nil { // 检查邮箱是否已存在
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeAlreadyExists, // 邮箱已存在错误代码
			Text: "邮箱已存在",
		}
	}

	// UIDVALIDITY 如果邮箱被删除再重新创建，必须更改
	u.prevUidValidity++
	u.mailboxes[name] = NewMailbox(name, u.prevUidValidity) // 创建新邮箱并保存
	return nil                                              // 返回 nil 表示成功
}

// Delete 方法删除指定的邮箱。
// 参数：
//   - name: 邮箱名称。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) Delete(name string) error {
	u.mutex.Lock()         // 锁定
	defer u.mutex.Unlock() // 解锁

	if _, err := u.mailboxLocked(name); err != nil { // 检查邮箱是否存在
		return err // 返回错误
	}

	delete(u.mailboxes, name) // 删除邮箱
	return nil                // 返回 nil 表示成功
}

// Rename 方法重命名指定的邮箱。
// 参数：
//   - oldName: 旧邮箱名称。
//   - newName: 新邮箱名称。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) Rename(oldName, newName string) error {
	u.mutex.Lock()         // 锁定
	defer u.mutex.Unlock() // 解锁

	newName = strings.TrimRight(newName, string(mailboxDelim)) // 去掉尾部的分隔符

	mbox, err := u.mailboxLocked(oldName) // 获取旧邮箱
	if err != nil {
		return err // 返回错误
	}

	if u.mailboxes[newName] != nil { // 检查新邮箱是否已存在
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeAlreadyExists, // 邮箱已存在错误代码
			Text: "邮箱已存在",
		}
	}

	mbox.rename(newName)         // 重命名邮箱
	u.mailboxes[newName] = mbox  // 更新邮箱映射
	delete(u.mailboxes, oldName) // 删除旧邮箱映射
	return nil                   // 返回 nil 表示成功
}

// Subscribe 方法订阅指定的邮箱。
// 参数：
//   - name: 邮箱名称。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) Subscribe(name string) error {
	mbox, err := u.mailbox(name) // 获取邮箱
	if err != nil {
		return err // 返回错误
	}
	mbox.SetSubscribed(true) // 设置为已订阅
	return nil               // 返回 nil 表示成功
}

// Unsubscribe 方法取消订阅指定的邮箱。
// 参数：
//   - name: 邮箱名称。
//
// 返回：
//   - 返回错误信息（如果有）。
func (u *User) Unsubscribe(name string) error {
	mbox, err := u.mailbox(name) // 获取邮箱
	if err != nil {
		return err // 返回错误
	}
	mbox.SetSubscribed(false) // 设置为未订阅
	return nil                // 返回 nil 表示成功
}

// Namespace 方法返回用户的命名空间信息。
// 返回：
//   - 返回命名空间数据和错误信息（如果有）。
func (u *User) Namespace() (*imap.NamespaceData, error) {
	return &imap.NamespaceData{
		Personal: []imap.NamespaceDescriptor{{Delim: mailboxDelim}}, // 返回个人命名空间描述
	}, nil
}
