package imap

// QuotaResourceType 表示 QUOTA 资源类型。
//
// 参见 RFC 9208 第 5 节。
type QuotaResourceType string

const (
	QuotaResourceStorage           QuotaResourceType = "STORAGE"            // 存储资源类型
	QuotaResourceMessage           QuotaResourceType = "MESSAGE"            // 消息资源类型
	QuotaResourceMailbox           QuotaResourceType = "MAILBOX"            // 邮箱资源类型
	QuotaResourceAnnotationStorage QuotaResourceType = "ANNOTATION-STORAGE" // 注释存储资源类型
)
