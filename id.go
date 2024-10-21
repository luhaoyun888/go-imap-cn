package imap

// IDData 表示客户端身份信息。
type IDData struct {
	Name        string // 客户端名称
	Version     string // 客户端版本
	OS          string // 操作系统名称
	OSVersion   string // 操作系统版本
	Vendor      string // 客户端供应商
	SupportURL  string // 支持链接
	Address     string // 客户端地址
	Date        string // 日期
	Command     string // 执行的命令
	Arguments   string // 命令参数
	Environment string // 环境信息
}
