package imap

import (
	"unsafe"

	"github.com/luhaoyun888/go-imap-cn/internal/imapnum"
)

// NumSet 是一组标识消息的数字。NumSet 可以是 SeqSet 或 UIDSet。
type NumSet interface {
	// String 返回消息编号集的 IMAP 表示。
	String() string
	// Dynamic 返回如果集合包含 "*" 或 "n:*" 范围，或者集合表示特殊的 SEARCHRES 标记时返回 true。
	Dynamic() bool

	numSet() imapnum.Set // 返回内部的 imapnum.Set
}

var (
	_ NumSet = SeqSet(nil) // 确保 SeqSet 实现了 NumSet 接口
	_ NumSet = UIDSet(nil) // 确保 UIDSet 实现了 NumSet 接口
)

// SeqSet 是一组消息序列号。
type SeqSet []SeqRange

// SeqSetNum 返回包含指定序列号的新 SeqSet。
func SeqSetNum(nums ...uint32) SeqSet {
	var s SeqSet
	s.AddNum(nums...) // 添加序列号
	return s
}

// numSetPtr 返回指向 imapnum.Set 的指针。
func (s *SeqSet) numSetPtr() *imapnum.Set {
	return (*imapnum.Set)(unsafe.Pointer(s))
}

// numSet 返回 SeqSet 对应的 imapnum.Set。
func (s SeqSet) numSet() imapnum.Set {
	return *s.numSetPtr()
}

// String 返回 SeqSet 的 IMAP 表示。
func (s SeqSet) String() string {
	return s.numSet().String()
}

// Dynamic 返回如果 SeqSet 是动态的，则返回 true。
func (s SeqSet) Dynamic() bool {
	return s.numSet().Dynamic()
}

// Contains 返回如果非零的序列号 num 包含在集合中则返回 true。
func (s *SeqSet) Contains(num uint32) bool {
	return s.numSet().Contains(num)
}

// Nums 返回包含在集合中的所有序列号的切片。
func (s *SeqSet) Nums() ([]uint32, bool) {
	return s.numSet().Nums() // 获取序列号列表
}

// AddNum 将新的序列号插入到集合中。值 0 表示 "*"。
func (s *SeqSet) AddNum(nums ...uint32) {
	s.numSetPtr().AddNum(nums...) // 添加序列号
}

// AddRange 将新的范围插入集合中。
func (s *SeqSet) AddRange(start, stop uint32) {
	s.numSetPtr().AddRange(start, stop) // 添加范围
}

// AddSet 将其他 SeqSet 的所有序列号插入到 s 中。
func (s *SeqSet) AddSet(other SeqSet) {
	s.numSetPtr().AddSet(other.numSet()) // 添加另一个集合的序列号
}

// SeqRange 是消息序列号的范围。
type SeqRange struct {
	Start, Stop uint32 // 范围的起始和结束序列号
}

// UIDSet 是一组消息 UID。
type UIDSet []UIDRange

// UIDSetNum 返回包含指定 UIDs 的新 UIDSet。
func UIDSetNum(uids ...UID) UIDSet {
	var s UIDSet
	s.AddNum(uids...) // 添加 UIDs
	return s
}

// numSetPtr 返回指向 imapnum.Set 的指针。
func (s *UIDSet) numSetPtr() *imapnum.Set {
	return (*imapnum.Set)(unsafe.Pointer(s))
}

// numSet 返回 UIDSet 对应的 imapnum.Set。
func (s UIDSet) numSet() imapnum.Set {
	return *s.numSetPtr()
}

// String 返回 UIDSet 的 IMAP 表示。如果是 SEARCHRES，返回 "$"。
func (s UIDSet) String() string {
	if IsSearchRes(s) { // 检查是否是搜索结果
		return "$"
	}
	return s.numSet().String() // 返回 UIDSet 的字符串表示
}

// Dynamic 返回如果 UIDSet 是动态的，则返回 true。
func (s UIDSet) Dynamic() bool {
	return s.numSet().Dynamic() || IsSearchRes(s) // 检查是否动态或是搜索结果
}

// Contains 返回如果非零的 UID uid 包含在集合中则返回 true。
func (s UIDSet) Contains(uid UID) bool {
	return s.numSet().Contains(uint32(uid)) // 检查 UID 是否在集合中
}

// Nums 返回包含在集合中的所有 UIDs 的切片。
func (s UIDSet) Nums() ([]UID, bool) {
	nums, ok := s.numSet().Nums() // 获取 UID 列表
	return uidListFromNumList(nums), ok
}

// AddNum 将新的 UIDs 插入到集合中。值 0 表示 "*"。
func (s *UIDSet) AddNum(uids ...UID) {
	s.numSetPtr().AddNum(numListFromUIDList(uids)...)
}

// AddRange 将新的范围插入集合中。
func (s *UIDSet) AddRange(start, stop UID) {
	s.numSetPtr().AddRange(uint32(start), uint32(stop)) // 添加范围
}

// AddSet 将其他 UIDSet 的所有 UIDs 插入到 s 中。
func (s *UIDSet) AddSet(other UIDSet) {
	s.numSetPtr().AddSet(other.numSet()) // 添加另一个集合的 UIDs
}

// UIDRange 是消息 UID 的范围。
type UIDRange struct {
	Start, Stop UID // 范围的起始和结束 UID
}

// numListFromUIDList 将 UID 列表转换为 uint32 列表。
func numListFromUIDList(uids []UID) []uint32 {
	return *(*[]uint32)(unsafe.Pointer(&uids)) // 使用 unsafe 包进行转换
}

// uidListFromNumList 将 uint32 列表转换为 UID 列表。
func uidListFromNumList(nums []uint32) []UID {
	return *(*[]UID)(unsafe.Pointer(&nums)) // 使用 unsafe 包进行转换
}
