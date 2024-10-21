package imap

// ListOptions 包含 LIST 命令的选项。
type ListOptions struct {
	SelectSubscribed     bool // 是否选择已订阅的邮箱
	SelectRemote         bool // 是否选择远程邮箱
	SelectRecursiveMatch bool // 是否选择递归匹配，需要设置 SelectSubscribed
	SelectSpecialUse     bool // 是否选择特殊用途邮箱，需要支持 SPECIAL-USE

	ReturnSubscribed bool           // 是否返回已订阅的邮箱
	ReturnChildren   bool           // 是否返回子邮箱
	ReturnStatus     *StatusOptions // 返回状态选项，要求 IMAP4rev2 或 LIST-STATUS
	ReturnSpecialUse bool           // 是否返回特殊用途邮箱，需要支持 SPECIAL-USE
}

// ListData 是 LIST 命令返回的邮箱数据。
type ListData struct {
	Attrs   []MailboxAttr // 邮箱属性的列表
	Delim   rune          // 用于分隔邮箱名称的分隔符
	Mailbox string        // 邮箱的名称

	// 扩展数据
	ChildInfo *ListDataChildInfo // 子邮箱信息
	OldName   string             // 旧的邮箱名称
	Status    *StatusData        // 状态数据
}

// ListDataChildInfo 是关于子邮箱的信息。
type ListDataChildInfo struct {
	Subscribed bool // 是否已订阅子邮箱
}
