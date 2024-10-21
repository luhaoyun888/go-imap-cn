package imap

import (
	"time"
)

// AppendOptions 包含 APPEND 命令的选项。
type AppendOptions struct {
	Flags []Flag    // 消息的标志，可以是多个 Flag 的组合
	Time  time.Time // 指定的时间，用于设置消息的时间戳
}

// AppendData 是 APPEND 命令返回的数据。
type AppendData struct {
	UID         UID    // 消息的唯一标识符，要求支持 UIDPLUS 或 IMAP4rev2
	UIDValidity uint32 // UID 的有效性，表示 UID 可能会在此有效性范围内变化
}
