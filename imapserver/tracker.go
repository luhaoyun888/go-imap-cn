package imapserver

import (
	"fmt"
	"sync"

	"github.com/emersion/go-imap/v2"
)

// MailboxTracker 用于跟踪邮箱的状态。
//
// 一个邮箱可以有多个会话监听更新。每个会话都有自己对邮箱的视图，
// 因为 IMAP 客户端异步接收邮箱更新。
type MailboxTracker struct {
	mutex       sync.Mutex // 互斥锁，用于保护对邮箱状态的并发访问
	numMessages uint32     // 当前邮件数量
	sessions    map[*SessionTracker]struct{} // 连接的会话列表
}

// NewMailboxTracker 创建一个新的邮箱跟踪器。
func NewMailboxTracker(numMessages uint32) *MailboxTracker {
	return &MailboxTracker{
		numMessages: numMessages,
		sessions:    make(map[*SessionTracker]struct{}),
	}
}

// NewSession 创建一个新的会话跟踪器，用于该邮箱。
//
// 调用者在完成会话后必须调用 SessionTracker.Close。
func (t *MailboxTracker) NewSession() *SessionTracker {
	st := &SessionTracker{mailbox: t} // 创建新的会话跟踪器
	t.mutex.Lock()
	t.sessions[st] = struct{}{} // 将新会话添加到会话列表
	t.mutex.Unlock()
	return st
}

// queueUpdate 将更新排入队列，通知其他会话。
func (t *MailboxTracker) queueUpdate(update *trackerUpdate, source *SessionTracker) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// 检查删除序号是否在范围内
	if update.expunge != 0 && update.expunge > t.numMessages {
		panic(fmt.Errorf("imapserver: 删除序号 (%v) 超出范围 (%v 邮件在邮箱中)", update.expunge, t.numMessages))
	}
	// 检查邮件数量是否递减
	if update.numMessages != 0 && update.numMessages < t.numMessages {
		panic(fmt.Errorf("imapserver: 不能将邮箱邮件数量从 %v 减少到 %v", t.numMessages, update.numMessages))
	}

	// 将更新通知给所有会话
	for st := range t.sessions {
		if source != nil && st == source {
			continue // 跳过源会话
		}
		st.queueUpdate(update)
	}

	// 更新邮箱邮件数量
	switch {
	case update.expunge != 0:
		t.numMessages-- // 删除邮件
	case update.numMessages != 0:
		t.numMessages = update.numMessages // 更新邮件数量
	}
}

// QueueExpunge 将新的 EXPUNGE 更新排入队列。
func (t *MailboxTracker) QueueExpunge(seqNum uint32) {
	if seqNum == 0 {
		panic("imapserver: 无效的删除邮件序号")
	}
	t.queueUpdate(&trackerUpdate{expunge: seqNum}, nil)
}

// QueueNumMessages 将新的 EXISTS 更新排入队列。
func (t *MailboxTracker) QueueNumMessages(n uint32) {
	// TODO: 合并连续的 NumMessages 更新
	t.queueUpdate(&trackerUpdate{numMessages: n}, nil)
}

// QueueMailboxFlags 将新的 FLAGS 更新排入队列。
func (t *MailboxTracker) QueueMailboxFlags(flags []imap.Flag) {
	if flags == nil {
		flags = []imap.Flag{} // 如果未提供标志，初始化为空
	}
	t.queueUpdate(&trackerUpdate{mailboxFlags: flags}, nil)
}

// QueueMessageFlags 将新的 FETCH FLAGS 更新排入队列。
//
// 如果 source 不为 nil，则该更新不会被分发给它。
func (t *MailboxTracker) QueueMessageFlags(seqNum uint32, uid imap.UID, flags []imap.Flag, source *SessionTracker) {
	t.queueUpdate(&trackerUpdate{fetch: &trackerUpdateFetch{
		seqNum: seqNum,
		uid:    uid,
		flags:  flags,
	}}, source)
}

// trackerUpdate 结构体用于跟踪邮箱的更新。
type trackerUpdate struct {
	expunge      uint32     // 要删除的邮件序号
	numMessages  uint32     // 当前邮件数量
	mailboxFlags []imap.Flag // 邮箱标志
	fetch        *trackerUpdateFetch // FETCH 更新
}

// trackerUpdateFetch 结构体用于跟踪邮件获取更新。
type trackerUpdateFetch struct {
	seqNum uint32     // 邮件序列号
	uid    imap.UID  // 邮件唯一标识符
	flags  []imap.Flag // 邮件标志
}

// SessionTracker 跟踪 IMAP 客户端的邮箱状态。
type SessionTracker struct {
	mailbox *MailboxTracker // 关联的邮箱跟踪器

	mutex   sync.Mutex      // 互斥锁，用于保护会话状态的并发访问
	queue   []trackerUpdate  // 待处理的更新队列
	updates chan<- struct{}  // 更新通知通道
}

// Close 注销会话。
func (t *SessionTracker) Close() {
	t.mailbox.mutex.Lock()
	delete(t.mailbox.sessions, t) // 从邮箱的会话列表中删除
	t.mailbox.mutex.Unlock()
	t.mailbox = nil // 清空邮箱引用
}

// queueUpdate 将更新排入会话的队列。
func (t *SessionTracker) queueUpdate(update *trackerUpdate) {
	var updates chan<- struct{}
	t.mutex.Lock()
	t.queue = append(t.queue, *update) // 将更新添加到队列
	updates = t.updates
	t.mutex.Unlock()

	if updates != nil {
		select {
		case updates <- struct{}{}: // 通知会话有新更新
			// 我们通知了 SessionTracker.Idle 有更新
		default:
			// 跳过更新
		}
	}
}

// Poll 从会话中取消排队的邮箱更新。
func (t *SessionTracker) Poll(w *UpdateWriter, allowExpunge bool) error {
	var updates []trackerUpdate
	t.mutex.Lock()
	if allowExpunge {
		updates = t.queue // 允许删除
		t.queue = nil // 清空队列
	} else {
		stopIndex := -1
		for i, update := range t.queue {
			if update.expunge != 0 { // 检查是否有删除更新
				stopIndex = i
				break
			}
			updates = append(updates, update) // 收集更新
		}
		if stopIndex >= 0 {
			t.queue = t.queue[stopIndex:] // 更新队列
		} else {
			t.queue = nil
		}
	}
	t.mutex.Unlock()

	// 写入更新到更新写入器
	for _, update := range updates {
		var err error
		switch {
		case update.expunge != 0:
			err = w.WriteExpunge(update.expunge) // 写入删除更新
		case update.numMessages != 0:
			err = w.WriteNumMessages(update.numMessages) // 写入邮件数量更新
		case update.mailboxFlags != nil:
			err = w.WriteMailboxFlags(update.mailboxFlags) // 写入邮箱标志更新
		case update.fetch != nil:
			err = w.WriteMessageFlags(update.fetch.seqNum, update.fetch.uid, update.fetch.flags) // 写入消息标志更新
		default:
			panic(fmt.Errorf("imapserver: 未知的跟踪更新 %#v", update))
		}
		if err != nil {
			return err // 返回错误
		}
	}
	return nil
}

// Idle 持续写入邮箱更新。
//
// 当停止通道关闭时，返回。
//
// Idle 不能从两个独立的 goroutine 同时调用。
func (t *SessionTracker) Idle(w *UpdateWriter, stop <-chan struct{}) error {
	updates := make(chan struct{}, 64) // 更新通知通道
	t.mutex.Lock()
	ok := t.updates == nil // 检查是否可以进行 Idle 调用
	if ok {
		t.updates = updates // 设置更新通道
	}
	t.mutex.Unlock()
	if !ok {
		return fmt.Errorf("imapserver: 同一时间只允许一个 SessionTracker.Idle 调用")
	}

	defer func() {
		t.mutex.Lock()
		t.updates = nil // 清空更新通道
		t.mutex.Unlock()
	}()

	for {
		select {
		case <-updates: // 收到更新通知
			if err := t.Poll(w, true); err != nil {
				return err
			}
		case <-stop: // 停止信号
			return nil
		}
	}
}

// DecodeSeqNum 将客户端视图的邮件序列号转换为服务器视图的序列号。
//
// 如果从服务器的角度看邮件不存在，则返回零。
func (t *SessionTracker) DecodeSeqNum(seqNum uint32) uint32 {
	if seqNum == 0 {
		return 0 // 序列号为零直接返回零
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, update := range t.queue {
		if update.expunge == 0 {
			continue
		}
		if seqNum == update.expunge { // 如果邮件被删除
			return 0
		} else if seqNum > update.expunge {
			seqNum-- // 减少序列号
		}
	}

	if seqNum > t.mailbox.numMessages {
		return 0 // 超出邮件数量
	}

	return seqNum
}

// EncodeSeqNum 将服务器视图的邮件序列号转换为客户端视图的序列号。
//
// 如果从客户端的角度看邮件不存在，则返回零。
func (t *SessionTracker) EncodeSeqNum(seqNum uint32) uint32 {
	if seqNum == 0 {
		return 0 // 序列号为零直接返回零
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	if seqNum > t.mailbox.numMessages {
		return 0 // 超出邮件数量
	}

	for i := len(t.queue) - 1; i >= 0; i-- {
		update := t.queue[i]
		// TODO: 这不处理递增大于1的情况
		if update.numMessages != 0 && seqNum == update.numMessages {
			return 0 // 如果邮件数量更新与当前序列号相等，返回零
		}
		if update.expunge != 0 && seqNum >= update.expunge {
			seqNum++ // 增加序列号
		}
	}
	return seqNum
}
