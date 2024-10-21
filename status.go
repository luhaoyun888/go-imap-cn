package imap

// StatusOptions 包含 STATUS 命令的选项。
type StatusOptions struct {
	NumMessages bool // 是否返回邮箱中的邮件数量
	UIDNext     bool // 是否返回下一个可用的 UID
	UIDValidity bool // 是否返回 UID 有效性
	NumUnseen   bool // 是否返回未读邮件数量
	NumDeleted  bool // 是否返回已删除邮件数量，要求 IMAP4rev2 或 QUOTA
	Size        bool // 是否返回邮箱大小，要求 IMAP4rev2 或 STATUS=SIZE

	AppendLimit    bool // 是否返回附加限制，要求 APPENDLIMIT
	DeletedStorage bool // 是否返回已删除邮件的存储量，要求 QUOTA=RES-STORAGE
	HighestModSeq  bool // 是否返回最高的修改序列号，要求 CONDSTORE
}

// StatusData 是 STATUS 命令返回的数据。
//
// 邮箱名称始终会填充，其他字段是可选的。
type StatusData struct {
	Mailbox string // 邮箱名称

	NumMessages *uint32 // 邮箱中的邮件数量
	UIDNext     UID     // 下一个可用的 UID
	UIDValidity uint32  // UID 有效性
	NumUnseen   *uint32 // 未读邮件数量
	NumDeleted  *uint32 // 已删除邮件数量
	Size        *int64  // 邮箱大小

	AppendLimit    *uint32 // 附加限制
	DeletedStorage *int64  // 已删除邮件的存储量
	HighestModSeq  uint64  // 最高的修改序列号
}
