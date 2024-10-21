package imap

// SelectOptions 包含 SELECT 或 EXAMINE 命令的选项。
type SelectOptions struct {
	ReadOnly  bool // 是否以只读模式选择邮箱
	CondStore bool // 是否使用条件存储，要求支持 CONDSTORE
}

// SelectData 是 SELECT 命令返回的数据。
//
// 在旧的 RFC 2060 中，PermanentFlags、UIDNext 和 UIDValidity 是可选的。
type SelectData struct {
	// 此邮箱定义的标志
	Flags []Flag // 邮箱的标志集合
	// 客户端可以永久更改的标志
	PermanentFlags []Flag // 客户端可永久更改的标志集合
	// 此邮箱中的邮件数量（即 "EXISTS"）
	NumMessages uint32 // 邮件总数
	UIDNext     UID    // 下一个 UID
	UIDValidity uint32 // UID 有效性

	List *ListData // 返回列表数据，要求支持 IMAP4rev2

	HighestModSeq uint64 // 最高的修改序列号，要求支持 CONDSTORE
}
