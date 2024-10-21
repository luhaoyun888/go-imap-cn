package imap

import (
	"fmt"
	"strings"
)

// IMAP4 ACL扩展 (RFC 2086)

// Right 描述了一组由 IMAP ACL 扩展控制的操作权限.
type Right byte

const (
	// 标准权限
	RightLookup     = Right('l') // 邮箱对 LIST/LSUB 命令可见
	RightRead       = Right('r') // 选择邮箱，执行 CHECK, FETCH, PARTIAL, SEARCH, COPY 从邮箱中
	RightSeen       = Right('s') // 跨会话保持已读/未读信息 (STORE SEEN 标志)
	RightWrite      = Right('w') // 除 SEEN 和 DELETED 之外的其他 STORE 标志
	RightInsert     = Right('i') // 执行 APPEND，COPY 到邮箱
	RightPost       = Right('p') // 发送邮件到邮箱的提交地址，IMAP4 本身不强制执行
	RightCreate     = Right('c') // 在任何实现定义的层次结构中创建新的子邮箱
	RightDelete     = Right('d') // STORE DELETED 标志，执行 EXPUNGE
	RightAdminister = Right('a') // 执行 SETACL
)

// RightSetAll 包含所有标准权限.
var RightSetAll = RightSet("lrswipcda")

// RightsIdentifier 是一个 ACL 标识符.
type RightsIdentifier string

// RightsIdentifierAnyone 是通用身份 (匹配所有人).
const RightsIdentifierAnyone = RightsIdentifier("anyone")

// NewRightsIdentifierUsername 返回一个引用用户名的权限标识符，检查保留值.
func NewRightsIdentifierUsername(username string) (RightsIdentifier, error) {
	// 检查用户名是否为保留标识符
	if username == string(RightsIdentifierAnyone) || strings.HasPrefix(username, "-") {
		return "", fmt.Errorf("imap: 保留的权限标识符")
	}
	return RightsIdentifier(username), nil
}

// RightModification 表示如何修改权限集.
type RightModification byte

const (
	RightModificationReplace = RightModification(0)   // 替换权限集
	RightModificationAdd     = RightModification('+') // 增加权限
	RightModificationRemove  = RightModification('-') // 删除权限
)

// RightSet 表示一组权限.
type RightSet []Right

// String 返回权限集的字符串表示形式.
func (r RightSet) String() string {
	return string(r)
}

// Add 返回一个新的权限集，包含两个权限集中的权限.
func (r RightSet) Add(rights RightSet) RightSet {
	// 创建一个新的权限集，长度为当前权限集的长度和传入的权限集长度之和
	newRights := make(RightSet, len(r), len(r)+len(rights))
	copy(newRights, r)

	// 将传入权限集中不在当前权限集中的权限添加到新权限集中
	for _, right := range rights {
		if !strings.ContainsRune(string(r), rune(right)) {
			newRights = append(newRights, right)
		}
	}

	return newRights
}

// Remove 返回一个新的权限集，包含 r 中所有不在传入权限集中的权限.
func (r RightSet) Remove(rights RightSet) RightSet {
	// 创建一个新的权限集，初始长度为当前权限集的长度
	newRights := make(RightSet, 0, len(r))

	// 将当前权限集中不在传入权限集中的权限添加到新权限集中
	for _, right := range r {
		if !strings.ContainsRune(string(rights), rune(right)) {
			newRights = append(newRights, right)
		}
	}

	return newRights
}

// Equal 返回 true 如果两个权限集包含完全相同的权限.
func (rs1 RightSet) Equal(rs2 RightSet) bool {
	// 检查 rs1 中的每个权限是否都在 rs2 中
	for _, r := range rs1 {
		if !strings.ContainsRune(string(rs2), rune(r)) {
			return false
		}
	}

	// 检查 rs2 中的每个权限是否都在 rs1 中
	for _, r := range rs2 {
		if !strings.ContainsRune(string(rs1), rune(r)) {
			return false
		}
	}

	return true
}
