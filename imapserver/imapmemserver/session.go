package imapmemserver

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

type (
	user    = User        // user 是 User 的别名
	mailbox = MailboxView // mailbox 是 MailboxView 的别名
)

// UserSession 表示与特定用户关联的会话。
//
// UserSession 实现了 imapserver.Session。通常，UserSession 指针
// 被嵌入到一个更大的结构体中，该结构体重写 Login 方法。
type UserSession struct {
	*user    // 不可变的用户指针
	*mailbox // 可为空的邮箱指针
}

var _ imapserver.SessionIMAP4rev2 = (*UserSession)(nil) // 确保 UserSession 实现了 SessionIMAP4rev2 接口

// NewUserSession 创建一个新的用户会话。
// 参数：
//   - user: 指向 User 的指针。
//
// 返回一个 UserSession 结构体指针。
func NewUserSession(user *User) *UserSession {
	return &UserSession{user: user}
}

// Close 方法关闭用户会话，并释放邮箱资源（如果存在）。
// 返回：
//   - 返回错误信息（如果有）。
func (sess *UserSession) Close() error {
	if sess != nil && sess.mailbox != nil {
		sess.mailbox.Close() // 关闭邮箱
	}
	return nil // 返回 nil 表示成功
}

// Select 方法选择指定的邮箱，并返回邮箱的选择数据。
// 参数：
//   - name: 邮箱名称。
//   - options: 选择选项。
//
// 返回：
//   - 返回选择数据和错误信息（如果有）。
func (sess *UserSession) Select(name string, options *imap.SelectOptions) (*imap.SelectData, error) {
	mbox, err := sess.user.mailbox(name) // 获取邮箱
	if err != nil {
		return nil, err // 返回错误
	}
	mbox.mutex.Lock()                   // 锁定邮箱
	defer mbox.mutex.Unlock()           // 解锁
	sess.mailbox = mbox.NewView()       // 创建邮箱视图
	return mbox.selectDataLocked(), nil // 返回选择数据
}

// Unselect 方法取消当前选择的邮箱。
// 返回：
//   - 返回错误信息（如果有）。
func (sess *UserSession) Unselect() error {
	sess.mailbox.Close() // 关闭当前邮箱
	sess.mailbox = nil   // 设置邮箱为 nil
	return nil           // 返回 nil 表示成功
}

// Copy 方法将指定邮件复制到目标邮箱。
// 参数：
//   - numSet: 要复制的邮件编号集合。
//   - destName: 目标邮箱名称。
//
// 返回：
//   - 返回复制数据和错误信息（如果有）。
func (sess *UserSession) Copy(numSet imap.NumSet, destName string) (*imap.CopyData, error) {
	dest, err := sess.user.mailbox(destName) // 获取目标邮箱
	if err != nil {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTryCreate, // 邮箱不存在，提示尝试创建
			Text: "找不到该邮箱",
		}
	} else if sess.mailbox != nil && dest == sess.mailbox.Mailbox {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "源邮箱和目标邮箱相同", // 源邮箱和目标邮箱相同
		}
	}

	var sourceUIDs, destUIDs imap.UIDSet // 源和目标邮箱的 UID 集合
	sess.mailbox.forEach(numSet, func(seqNum uint32, msg *message) {
		appendData := dest.copyMsg(msg) // 复制邮件
		sourceUIDs.AddNum(msg.uid)      // 添加源 UID
		destUIDs.AddNum(appendData.UID) // 添加目标 UID
	})

	return &imap.CopyData{
		UIDValidity: dest.uidValidity, // 返回目标邮箱的 UID 有效性
		SourceUIDs:  sourceUIDs,       // 返回源 UID 集合
		DestUIDs:    destUIDs,         // 返回目标 UID 集合
	}, nil
}

// Move 方法将指定邮件移动到目标邮箱。
// 参数：
//   - w: MoveWriter，用于写入结果。
//   - numSet: 要移动的邮件编号集合。
//   - destName: 目标邮箱名称。
//
// 返回：
//   - 返回错误信息（如果有）。
func (sess *UserSession) Move(w *imapserver.MoveWriter, numSet imap.NumSet, destName string) error {
	dest, err := sess.user.mailbox(destName) // 获取目标邮箱
	if err != nil {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTryCreate, //邮箱不存在 ，提示尝试创建
			Text: "找不到该邮箱",
		}
	} else if sess.mailbox != nil && dest == sess.mailbox.Mailbox {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "源邮箱和目标邮箱相同", // 源邮箱和目标邮箱相同
		}
	}

	sess.mailbox.mutex.Lock()         // 锁定源邮箱
	defer sess.mailbox.mutex.Unlock() // 解锁

	var sourceUIDs, destUIDs imap.UIDSet    // 源和目标邮箱的 UID 集合
	expunged := make(map[*message]struct{}) // 存储被删除的邮件
	sess.mailbox.forEachLocked(numSet, func(seqNum uint32, msg *message) {
		appendData := dest.copyMsg(msg) // 复制邮件
		sourceUIDs.AddNum(msg.uid)      // 添加源 UID
		destUIDs.AddNum(appendData.UID) // 添加目标 UID
		expunged[msg] = struct{}{}      // 标记为被删除
	})
	seqNums := sess.mailbox.expungeLocked(expunged) // 清理已删除邮件

	err = w.WriteCopyData(&imap.CopyData{
		UIDValidity: dest.uidValidity, // 返回目标邮箱的 UID 有效性
		SourceUIDs:  sourceUIDs,       // 返回源 UID 集合
		DestUIDs:    destUIDs,         // 返回目标 UID 集合
	})
	if err != nil {
		return err // 返回错误
	}

	for _, seqNum := range seqNums { // 遍历已删除邮件的序号
		if err := w.WriteExpunge(sess.mailbox.tracker.EncodeSeqNum(seqNum)); err != nil {
			return err // 返回错误
		}
	}

	return nil // 返回 nil 表示成功
}

// Poll 方法从当前邮箱中轮询更新。
// 参数：
//   - w: UpdateWriter，用于写入更新结果。
//   - allowExpunge: 是否允许清理已删除邮件。
//
// 返回：
//   - 返回错误信息（如果有）。
func (sess *UserSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	if sess.mailbox == nil {
		return nil // 如果没有邮箱，返回 nil
	}
	return sess.mailbox.Poll(w, allowExpunge) // 调用邮箱的 Poll 方法
}

// Idle 方法使会话进入闲置状态，等待更新。
// 参数：
//   - w: UpdateWriter，用于写入更新结果。
//   - stop: 用于停止闲置的通道。
//
// 返回：
//   - 返回错误信息（如果有）。
func (sess *UserSession) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	if sess.mailbox == nil {
		return nil // 如果没有邮箱，返回 nil
	}
	return sess.mailbox.Idle(w, stop) // 调用邮箱的 Idle 方法
}
