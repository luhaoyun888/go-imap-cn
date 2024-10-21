package imap

// StoreOptions 包含 STORE 命令的选项。
type StoreOptions struct {
	UnchangedSince uint64 // 要求 CONDSTORE
}

// StoreFlagsOp 是标志操作：设置、添加或删除。
type StoreFlagsOp int

const (
	StoreFlagsSet StoreFlagsOp = iota // 设置标志
	StoreFlagsAdd                     // 添加标志
	StoreFlagsDel                     // 删除标志
)

// StoreFlags 修改消息标志。
type StoreFlags struct {
	Op     StoreFlagsOp // 操作类型
	Silent bool         // 是否静默操作
	Flags  []Flag       // 要修改的标志
}
