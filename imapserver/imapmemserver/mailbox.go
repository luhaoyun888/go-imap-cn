package imapmemserver

import (
	"bytes"
	"sort"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

// Mailbox 是一个内存中的邮箱。
// 同一个邮箱可以在多个连接和多个用户之间共享。
type Mailbox struct {
	tracker     *imapserver.MailboxTracker // 邮箱跟踪器，用于跟踪邮箱的状态
	uidValidity uint32                     // UID 有效性，用于确保 UID 的唯一性

	mutex      sync.Mutex // 互斥锁，用于保护邮箱的并发访问
	name       string     // 邮箱名称
	subscribed bool       // 是否订阅该邮箱
	l          []*message // 存储邮件的切片
	uidNext    imap.UID   // 下一个 UID
}

// NewMailbox 创建一个新的邮箱。
func NewMailbox(name string, uidValidity uint32) *Mailbox {
	return &Mailbox{
		tracker:     imapserver.NewMailboxTracker(0), // 初始化邮箱跟踪器
		uidValidity: uidValidity,                     // 设置 UID 有效性
		name:        name,                            // 设置邮箱名称
		uidNext:     1,                               // 初始化下一个 UID 为 1
	}
}

// list 返回邮箱的列表数据。
// options: 列表选项，包括是否选择已订阅的邮箱。
func (mbox *Mailbox) list(options *imap.ListOptions) *imap.ListData {
	mbox.mutex.Lock()
	defer mbox.mutex.Unlock()

	if options.SelectSubscribed && !mbox.subscribed { // 如果选择已订阅的邮箱但当前未订阅，则返回 nil
		return nil
	}

	data := imap.ListData{
		Mailbox: mbox.name,    // 设置邮箱名称
		Delim:   mailboxDelim, // 设置邮箱分隔符
	}
	if mbox.subscribed { // 如果已订阅，添加订阅属性
		data.Attrs = append(data.Attrs, imap.MailboxAttrSubscribed)
	}
	if options.ReturnStatus != nil { // 如果请求状态信息，获取状态数据
		data.Status = mbox.statusDataLocked(options.ReturnStatus)
	}
	return &data
}

// StatusData 返回 STATUS 命令的数据。
// options: 状态选项，指定需要返回的状态信息。
func (mbox *Mailbox) StatusData(options *imap.StatusOptions) *imap.StatusData {
	mbox.mutex.Lock()
	defer mbox.mutex.Unlock()
	return mbox.statusDataLocked(options)
}

// statusDataLocked 在锁定状态下返回邮箱状态数据。
// options: 状态选项，指定需要返回的状态信息。
func (mbox *Mailbox) statusDataLocked(options *imap.StatusOptions) *imap.StatusData {
	data := imap.StatusData{Mailbox: mbox.name} // 创建状态数据，设置邮箱名称
	if options.NumMessages {                    // 如果请求消息数量
		num := uint32(len(mbox.l)) // 获取邮件数量
		data.NumMessages = &num    // 设置邮件数量
	}
	if options.UIDNext { // 如果请求下一个 UID
		data.UIDNext = mbox.uidNext // 设置下一个 UID
	}
	if options.UIDValidity { // 如果请求 UID 有效性
		data.UIDValidity = mbox.uidValidity // 设置 UID 有效性
	}
	if options.NumUnseen { // 如果请求未读邮件数量
		num := uint32(len(mbox.l)) - mbox.countByFlagLocked(imap.FlagSeen) // 计算未读邮件数量
		data.NumUnseen = &num                                              // 设置未读邮件数量
	}
	if options.NumDeleted { // 如果请求已删除邮件数量
		num := mbox.countByFlagLocked(imap.FlagDeleted) // 计算已删除邮件数量
		data.NumDeleted = &num                          // 设置已删除邮件数量
	}
	if options.Size { // 如果请求邮件总大小
		size := mbox.sizeLocked() // 计算邮件总大小
		data.Size = &size         // 设置邮件总大小
	}
	return &data
}

// countByFlagLocked 在锁定状态下计算具有指定标志的邮件数量。
// flag: 要计数的邮件标志。
func (mbox *Mailbox) countByFlagLocked(flag imap.Flag) uint32 {
	var n uint32
	for _, msg := range mbox.l { // 遍历所有邮件
		if _, ok := msg.flags[canonicalFlag(flag)]; ok { // 如果邮件具有指定标志
			n++ // 增加计数
		}
	}
	return n
}

// sizeLocked 在锁定状态下计算邮件总大小。
func (mbox *Mailbox) sizeLocked() int64 {
	var size int64
	for _, msg := range mbox.l { // 遍历所有邮件
		size += int64(len(msg.buf)) // 累加邮件大小
	}
	return size
}

// appendLiteral 将字面量内容附加到邮箱中。
// r: 邮件内容的字面量读取器，options: 附加选项。
func (mbox *Mailbox) appendLiteral(r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil { // 从读取器中读取字面量内容
		return nil, err // 如果出错，返回错误
	}
	return mbox.appendBytes(buf.Bytes(), options), nil // 将字节内容附加到邮箱
}

// copyMsg 复制一封邮件并返回附加数据。
// msg: 要复制的邮件。
func (mbox *Mailbox) copyMsg(msg *message) *imap.AppendData {
	return mbox.appendBytes(msg.buf, &imap.AppendOptions{
		Time:  msg.t,          // 邮件时间
		Flags: msg.flagList(), // 邮件标志
	})
}

// appendBytes 将字节内容附加到邮箱中。
// buf: 邮件内容的字节切片，options: 附加选项。
func (mbox *Mailbox) appendBytes(buf []byte, options *imap.AppendOptions) *imap.AppendData {
	msg := &message{
		flags: make(map[imap.Flag]struct{}), // 初始化邮件标志
		buf:   buf,                          // 设置邮件内容
	}

	if options.Time.IsZero() { // 如果未指定时间，则使用当前时间
		msg.t = time.Now()
	} else {
		msg.t = options.Time // 否则使用指定时间
	}

	for _, flag := range options.Flags { // 设置邮件标志
		msg.flags[canonicalFlag(flag)] = struct{}{}
	}

	mbox.mutex.Lock() // 锁定邮箱以进行并发安全访问
	defer mbox.mutex.Unlock()

	msg.uid = mbox.uidNext // 设置邮件 UID
	mbox.uidNext++         // 更新下一个 UID

	mbox.l = append(mbox.l, msg)                       // 将邮件添加到邮箱中
	mbox.tracker.QueueNumMessages(uint32(len(mbox.l))) // 更新消息数量

	return &imap.AppendData{
		UIDValidity: mbox.uidValidity, // 返回 UID 有效性
		UID:         msg.uid,          // 返回邮件 UID
	}
}

// rename 更改邮箱名称。
// newName: 新的邮箱名称。
func (mbox *Mailbox) rename(newName string) {
	mbox.mutex.Lock()   // 锁定邮箱以进行并发安全访问
	mbox.name = newName // 更新邮箱名称
	mbox.mutex.Unlock() // 解锁
}

// SetSubscribed 更改邮箱的订阅状态。
// subscribed: 订阅状态，true 表示订阅，false 表示未订阅。
func (mbox *Mailbox) SetSubscribed(subscribed bool) {
	mbox.mutex.Lock()            // 锁定邮箱以进行并发安全访问
	mbox.subscribed = subscribed // 更新订阅状态
	mbox.mutex.Unlock()          // 解锁
}

// selectDataLocked 在锁定状态下返回选择数据。
func (mbox *Mailbox) selectDataLocked() *imap.SelectData {
	flags := mbox.flagsLocked() // 获取当前邮件标志

	permanentFlags := make([]imap.Flag, len(flags))            // 创建一个永久标志的切片
	copy(permanentFlags, flags)                                // 复制当前邮件标志
	permanentFlags = append(permanentFlags, imap.FlagWildcard) // 添加通配符标志

	return &imap.SelectData{
		Flags:          flags,               // 返回当前标志
		PermanentFlags: permanentFlags,      // 返回永久标志
		NumMessages:    uint32(len(mbox.l)), // 返回邮件数量
		UIDNext:        mbox.uidNext,        // 返回下一个 UID
		UIDValidity:    mbox.uidValidity,    // 返回 UID 有效性
	}
}

// flagsLocked 在锁定状态下返回所有邮件的标志。
func (mbox *Mailbox) flagsLocked() []imap.Flag {
	m := make(map[imap.Flag]struct{}) // 使用 map 存储唯一的标志
	for _, msg := range mbox.l {      // 遍历邮箱中的所有邮件
		for flag := range msg.flags { // 遍历邮件的标志
			m[flag] = struct{}{} // 将标志添加到 map 中，确保唯一性
		}
	}

	var l []imap.Flag     // 创建切片以存储唯一标志
	for flag := range m { // 遍历 map 中的标志
		l = append(l, flag) // 将标志添加到切片
	}

	sort.Slice(l, func(i, j int) bool { // 对标志进行排序
		return l[i] < l[j]
	})

	return l // 返回标志切片
}

// Expunge 删除已标记为删除的邮件。
// w: 用于写入的 ExpungeWriter，uids: 要删除的邮件的 UID 集。
func (mbox *Mailbox) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	expunged := make(map[*message]struct{}) // 存储待删除的邮件
	mbox.mutex.Lock()                       // 锁定邮箱以进行并发安全访问
	for _, msg := range mbox.l {            // 遍历所有邮件
		if uids != nil && !uids.Contains(msg.uid) { // 如果指定了 UID 集并且当前邮件不在其中，则跳过
			continue
		}
		if _, ok := msg.flags[canonicalFlag(imap.FlagDeleted)]; ok { // 如果邮件标记为已删除
			expunged[msg] = struct{}{} // 将邮件添加到待删除集合中
		}
	}
	mbox.mutex.Unlock() // 解锁

	if len(expunged) == 0 { // 如果没有待删除的邮件
		return nil // 返回 nil
	}

	mbox.mutex.Lock()            // 锁定邮箱以进行并发安全访问
	mbox.expungeLocked(expunged) // 调用内部方法删除邮件
	mbox.mutex.Unlock()          // 解锁

	return nil // 返回 nil
}

// expungeLocked 在锁定状态下删除已标记为删除的邮件。
// expunged: 待删除的邮件集合。
func (mbox *Mailbox) expungeLocked(expunged map[*message]struct{}) (seqNums []uint32) {
	// TODO: 优化

	// 反向迭代，以保持序列号的一致性
	var filtered []*message
	for i := len(mbox.l) - 1; i >= 0; i-- { // 从最后一封邮件开始迭代
		msg := mbox.l[i]
		if _, ok := expunged[msg]; ok { // 如果当前邮件在待删除集合中
			seqNum := uint32(i) + 1           // 计算序列号
			seqNums = append(seqNums, seqNum) // 将序列号添加到返回切片中
			mbox.tracker.QueueExpunge(seqNum) // 更新跟踪器以通知删除
		} else {
			filtered = append(filtered, msg) // 如果邮件未被删除，添加到过滤后的切片中
		}
	}

	// 反转过滤后的切片
	for i := 0; i < len(filtered)/2; i++ {
		j := len(filtered) - i - 1
		filtered[i], filtered[j] = filtered[j], filtered[i] // 反转切片顺序
	}

	mbox.l = filtered // 更新邮箱中的邮件列表

	return seqNums // 返回已删除邮件的序列号
}

// NewView 创建一个新的邮箱视图。
// 调用者必须在使用完邮箱视图后调用 MailboxView.Close。
func (mbox *Mailbox) NewView() *MailboxView {
	return &MailboxView{
		Mailbox: mbox,                      // 关联当前邮箱
		tracker: mbox.tracker.NewSession(), // 创建新的会话跟踪器
	}
}

// MailboxView 是一个邮箱的视图。
// 每个视图都有自己的一组待处理的单方面更新。
// 当邮箱视图不再使用时，必须调用 Close。
// 通常，为每个在选定状态下的 IMAP 连接创建新的 MailboxView。
type MailboxView struct {
	*Mailbox                             // 嵌入 Mailbox
	tracker   *imapserver.SessionTracker // 会话跟踪器
	searchRes imap.UIDSet                // 搜索结果的 UID 集
}

// Close 释放为邮箱视图分配的资源。
func (mbox *MailboxView) Close() {
	mbox.tracker.Close() // 关闭跟踪器
}

// Fetch 获取邮件数据。
// w: 用于写入的 FetchWriter，numSet: 要获取的邮件序列号集合，options: 获取选项。
func (mbox *MailboxView) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	markSeen := false                        // 标记是否需要将邮件标记为已读
	for _, bs := range options.BodySection { // 遍历请求的邮件体部分
		if !bs.Peek { // 如果不是只查看标记
			markSeen = true // 标记为已读
			break
		}
	}

	var err error
	mbox.forEach(numSet, func(seqNum uint32, msg *message) { // 遍历要获取的邮件
		if err != nil {
			return // 如果出错，停止遍历
		}

		if markSeen { // 如果需要标记为已读
			msg.flags[canonicalFlag(imap.FlagSeen)] = struct{}{}                         // 设置已读标志
			mbox.Mailbox.tracker.QueueMessageFlags(seqNum, msg.uid, msg.flagList(), nil) // 更新标志到跟踪器
		}

		respWriter := w.CreateMessage(mbox.tracker.EncodeSeqNum(seqNum)) // 创建响应写入器
		err = msg.fetch(respWriter, options)                             // 获取邮件数据
	})
	return err // 返回可能的错误
}

// Search 在邮箱中搜索符合条件的邮件。
// numKind: 序列号或 UID 类型，criteria: 搜索条件，options: 搜索选项。
func (mbox *MailboxView) Search(numKind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	mbox.mutex.Lock() // 锁定邮箱以进行并发安全访问
	defer mbox.mutex.Unlock()

	mbox.staticSearchCriteria(criteria) // 处理静态搜索条件

	data := imap.SearchData{UID: numKind == imapserver.NumKindUID} // 初始化搜索数据

	var (
		seqSet imap.SeqSet // 序列号集合
		uidSet imap.UIDSet // UID 集合
	)
	for i, msg := range mbox.l { // 遍历邮箱中的所有邮件
		seqNum := mbox.tracker.EncodeSeqNum(uint32(i) + 1) // 计算序列号

		if !msg.search(seqNum, criteria) { // 如果邮件不符合搜索条件
			continue // 跳过
		}

		// 始终填充 UID 集合，因为它可能稍后保存用于 SEARCHRES
		uidSet.AddNum(msg.uid)

		var num uint32
		switch numKind { // 根据 numKind 计算序列号或 UID
		case imapserver.NumKindSeq:
			if seqNum == 0 {
				continue
			}
			seqSet.AddNum(seqNum)
			num = seqNum
		case imapserver.NumKindUID:
			num = uint32(msg.uid)
		}
		if data.Min == 0 || num < data.Min {
			data.Min = num // 更新最小值
		}
		if data.Max == 0 || num > data.Max {
			data.Max = num // 更新最大值
		}
		data.Count++ // 增加计数
	}

	switch numKind {
	case imapserver.NumKindSeq:
		data.All = seqSet // 设置结果集合为序列号集合
	case imapserver.NumKindUID:
		data.All = uidSet // 设置结果集合为 UID 集合
	}

	if options.ReturnSave { // 如果请求返回保存的 UID 集合
		mbox.searchRes = uidSet // 更新搜索结果
	}

	return &data, nil // 返回搜索数据
}

// staticSearchCriteria 处理静态搜索条件。
// criteria: 搜索条件。
func (mbox *MailboxView) staticSearchCriteria(criteria *imap.SearchCriteria) {
	seqNums := make([]imap.SeqSet, 0, len(criteria.SeqNum)) // 存储序列号集合
	for _, seqSet := range criteria.SeqNum {                // 遍历搜索条件中的序列号集合
		numSet := mbox.staticNumSet(seqSet) // 转换为静态集合
		switch numSet := numSet.(type) {
		case imap.SeqSet:
			seqNums = append(seqNums, numSet) // 添加到序列号集合
		case imap.UIDSet: // 如果是 UID 集合（可能在 SEARCHRES 中出现）
			criteria.UID = append(criteria.UID, numSet) // 添加到 UID 集合
		}
	}
	criteria.SeqNum = seqNums // 更新搜索条件中的序列号集合

	for i, uidSet := range criteria.UID { // 遍历 UID 集合
		criteria.UID[i] = mbox.staticNumSet(uidSet).(imap.UIDSet) // 转换为静态 UID 集合
	}

	for i := range criteria.Not { // 处理 NOT 条件
		mbox.staticSearchCriteria(&criteria.Not[i]) // 递归处理
	}
	for i := range criteria.Or { // 处理 OR 条件
		for j := range criteria.Or[i] {
			mbox.staticSearchCriteria(&criteria.Or[i][j]) // 递归处理
		}
	}
}

// Store 存储邮件的标志。
// w: 用于写入的 FetchWriter，numSet: 要更新的邮件序列号集合，flags: 要更新的标志，options: 存储选项。
func (mbox *MailboxView) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	mbox.forEach(numSet, func(seqNum uint32, msg *message) { // 遍历要更新的邮件
		msg.store(flags)                                                                      // 存储标志
		mbox.Mailbox.tracker.QueueMessageFlags(seqNum, msg.uid, msg.flagList(), mbox.tracker) // 更新到跟踪器
	})
	if !flags.Silent { // 如果不是静默模式
		return mbox.Fetch(w, numSet, &imap.FetchOptions{Flags: true}) // 获取更新后的邮件数据
	}
	return nil // 返回 nil
}

// Poll 检查邮箱更新。
// w: 用于写入的 UpdateWriter，allowExpunge: 是否允许删除操作。
func (mbox *MailboxView) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	return mbox.tracker.Poll(w, allowExpunge) // 使用跟踪器检查更新
}

// Idle 进入空闲状态。
// w: 用于写入的 UpdateWriter，stop: 停止信号通道。
func (mbox *MailboxView) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	return mbox.tracker.Idle(w, stop) // 使用跟踪器进入空闲状态
}

// forEach 遍历邮件集合，并对每封邮件执行操作。
// numSet: 要遍历的邮件序列号集合，f: 处理函数。
func (mbox *MailboxView) forEach(numSet imap.NumSet, f func(seqNum uint32, msg *message)) {
	mbox.mutex.Lock()         // 锁定邮箱以进行并发安全访问
	defer mbox.mutex.Unlock() // 解锁

	mbox.forEachLocked(numSet, f) // 调用锁定的方法
}

// forEachLocked 在锁定状态下遍历邮件集合，并对每封邮件执行操作。
// numSet: 要遍历的邮件序列号集合，f: 处理函数。
func (mbox *MailboxView) forEachLocked(numSet imap.NumSet, f func(seqNum uint32, msg *message)) {
	// TODO: 优化

	numSet = mbox.staticNumSet(numSet) // 转换为静态集合

	for i, msg := range mbox.l { // 遍历邮箱中的所有邮件
		seqNum := uint32(i) + 1 // 计算序列号

		var contains bool
		switch numSet := numSet.(type) {
		case imap.SeqSet: // 如果是序列号集合
			seqNum := mbox.tracker.EncodeSeqNum(seqNum)       // 编码序列号
			contains = seqNum != 0 && numSet.Contains(seqNum) // 检查是否包含在集合中
		case imap.UIDSet: // 如果是 UID 集合
			contains = numSet.Contains(msg.uid) // 检查是否包含在集合中
		}
		if !contains { // 如果不包含
			continue // 跳过
		}

		f(seqNum, msg) // 调用处理函数
	}
}

// staticNumSet 将动态序列号集合转换为静态集合。
// 这对于正确处理特殊符号 "*"（表示邮箱中的最大序列号或 UID）是必要的。
// 此函数还处理特殊的 SEARCHRES 标记 "$"。
func (mbox *MailboxView) staticNumSet(numSet imap.NumSet) imap.NumSet {
	if imap.IsSearchRes(numSet) { // 检查是否为搜索结果
		return mbox.searchRes // 返回搜索结果
	}

	switch numSet := numSet.(type) {
	case imap.SeqSet: // 如果是序列号集合
		max := uint32(len(mbox.l)) // 获取最大序列号
		for i := range numSet {
			r := &numSet[i]
			staticNumRange(&r.Start, &r.Stop, max) // 转换为静态范围
		}
	case imap.UIDSet: // 如果是 UID 集合
		max := uint32(mbox.uidNext) - 1 // 获取最大 UID
		for i := range numSet {
			r := &numSet[i]
			staticNumRange((*uint32)(&r.Start), (*uint32)(&r.Stop), max) // 转换为静态范围
		}
	}

	return numSet // 返回转换后的集合
}

// staticNumRange 将动态范围转换为静态范围。
func staticNumRange(start, stop *uint32, max uint32) {
	dyn := false
	if *start == 0 { // 如果起始值为 0
		*start = max // 设置为最大值
		dyn = true
	}
	if *stop == 0 { // 如果结束值为 0
		*stop = max // 设置为最大值
		dyn = true
	}
	if dyn && *start > *stop { // 如果动态范围且起始值大于结束值
		*start, *stop = *stop, *start // 交换起始值和结束值
	}
}
