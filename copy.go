package imap

// CopyData 是 COPY 命令返回的数据。
type CopyData struct {
	UIDValidity uint32 // UID 的有效性，要求支持 UIDPLUS 或 IMAP4rev2
	SourceUIDs  UIDSet // 源 UID 集，表示被复制邮件的 UID 集合
	DestUIDs    UIDSet // 目标 UID 集，表示复制后邮件在目标邮箱中的 UID 集合
}
