package imap

// NamespaceData 是 NAMESPACE 命令返回的数据。
type NamespaceData struct {
	Personal []NamespaceDescriptor // 用户个人命名空间的描述
	Other    []NamespaceDescriptor // 其他命名空间的描述
	Shared   []NamespaceDescriptor // 共享命名空间的描述
}

// NamespaceDescriptor 描述一个命名空间。
type NamespaceDescriptor struct {
	Prefix string // 命名空间的前缀
	Delim  rune   // 命名空间的分隔符
}
