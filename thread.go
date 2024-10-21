package imap

// ThreadAlgorithm 表示一个线程算法。
type ThreadAlgorithm string

const (
	ThreadOrderedSubject ThreadAlgorithm = "ORDEREDSUBJECT" // 有序主题算法
	ThreadReferences     ThreadAlgorithm = "REFERENCES"     // 引用算法
)
