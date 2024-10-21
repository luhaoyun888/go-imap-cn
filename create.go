package imap

// CreateOptions 包含 CREATE 命令的选项。
type CreateOptions struct {
	SpecialUse []MailboxAttr // 特殊用途属性，要求支持 CREATE-SPECIAL-USE
}
