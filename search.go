package imap

import (
	"reflect"
	"time"
)

// SearchOptions 包含 SEARCH 命令的选项。
type SearchOptions struct {
	// 需要 IMAP4rev2 或 ESEARCH
	ReturnMin   bool // 返回最小值
	ReturnMax   bool // 返回最大值
	ReturnAll   bool // 返回所有结果
	ReturnCount bool // 返回计数
	// 需要 IMAP4rev2 或 SEARCHRES
	ReturnSave bool // 保存搜索结果
}

// SearchCriteria 表示 SEARCH 命令的搜索条件。
//
// 当多个字段被填充时，结果是符合所有条件消息的交集（"与" 操作）。
//
// "And", "Not" 和 "Or" 可以用来组合多个搜索条件。例如，以下条件匹配不包含 "hello" 的消息：
//
//	SearchCriteria{Not: []SearchCriteria{{
//		Body: []string{"hello"},
//	}}}
//
// 以下条件匹配包含 "hello" 或 "world" 的消息：
//
//	SearchCriteria{Or: [][2]SearchCriteria{{
//		{Body: []string{"hello"}},
//		{Body: []string{"world"}},
//	}}}
type SearchCriteria struct {
	SeqNum []SeqSet // 消息序号
	UID    []UIDSet // 消息UID

	// 仅使用日期，时间和时区被忽略
	Since      time.Time // 自某日期以来
	Before     time.Time // 在某日期之前
	SentSince  time.Time // 自某日期以来发送
	SentBefore time.Time // 在某日期之前发送

	Header []SearchCriteriaHeaderField // 邮件头字段
	Body   []string                    // 邮件正文内容
	Text   []string                    // 邮件文本内容

	Flag    []Flag // 含有的标志
	NotFlag []Flag // 不含有的标志

	Larger  int64 // 大于某个大小
	Smaller int64 // 小于某个大小

	Not []SearchCriteria    // 否定的搜索条件
	Or  [][2]SearchCriteria // "或" 条件组合

	ModSeq *SearchCriteriaModSeq // 条件存储功能（需要 CONDSTORE 扩展）
}

// And 方法用于合并两个搜索条件的交集。
//
// 参数：
// - other: 另一个要合并的搜索条件。
func (criteria *SearchCriteria) And(other *SearchCriteria) {
	criteria.SeqNum = append(criteria.SeqNum, other.SeqNum...)
	criteria.UID = append(criteria.UID, other.UID...)

	criteria.Since = intersectSince(criteria.Since, other.Since)
	criteria.Before = intersectBefore(criteria.Before, other.Before)
	criteria.SentSince = intersectSince(criteria.SentSince, other.SentSince)
	criteria.SentBefore = intersectBefore(criteria.SentBefore, other.SentBefore)

	criteria.Header = append(criteria.Header, other.Header...)
	criteria.Body = append(criteria.Body, other.Body...)
	criteria.Text = append(criteria.Text, other.Text...)

	criteria.Flag = append(criteria.Flag, other.Flag...)
	criteria.NotFlag = append(criteria.NotFlag, other.NotFlag...)

	// 合并 Larger 和 Smaller 条件
	if criteria.Larger == 0 || other.Larger > criteria.Larger {
		criteria.Larger = other.Larger
	}
	if criteria.Smaller == 0 || other.Smaller < criteria.Smaller {
		criteria.Smaller = other.Smaller
	}

	criteria.Not = append(criteria.Not, other.Not...)
	criteria.Or = append(criteria.Or, other.Or...)
}

// intersectSince 方法用于返回两个日期中较晚的日期。
//
// 参数：
// - t1: 第一个日期。
// - t2: 第二个日期。
//
// 返回：
// 较晚的日期。
func intersectSince(t1, t2 time.Time) time.Time {
	switch {
	case t1.IsZero(): // 如果 t1 未设置，返回 t2
		return t2
	case t2.IsZero(): // 如果 t2 未设置，返回 t1
		return t1
	case t1.After(t2): // 返回较晚的日期
		return t1
	default:
		return t2
	}
}

// intersectBefore 方法用于返回两个日期中较早的日期。
//
// 参数：
// - t1: 第一个日期。
// - t2: 第二个日期。
//
// 返回：
// 较早的日期。
func intersectBefore(t1, t2 time.Time) time.Time {
	switch {
	case t1.IsZero(): // 如果 t1 未设置，返回 t2
		return t2
	case t2.IsZero(): // 如果 t2 未设置，返回 t1
		return t1
	case t1.Before(t2): // 返回较早的日期
		return t1
	default:
		return t2
	}
}

// SearchCriteriaHeaderField 表示邮件头的键值对字段。
type SearchCriteriaHeaderField struct {
	Key, Value string // 键和值
}

// SearchCriteriaModSeq 表示 ModSeq 条件。
//
// 字段：
// - ModSeq: 条件的 ModSeq 值。
// - MetadataName: 元数据名称。
// - MetadataType: 元数据类型。
type SearchCriteriaModSeq struct {
	ModSeq       uint64
	MetadataName string
	MetadataType SearchCriteriaMetadataType
}

// SearchCriteriaMetadataType 表示元数据类型。
type SearchCriteriaMetadataType string

const (
	SearchCriteriaMetadataAll     SearchCriteriaMetadataType = "所有"
	SearchCriteriaMetadataPrivate SearchCriteriaMetadataType = "私人"
	SearchCriteriaMetadataShared  SearchCriteriaMetadataType = "共享"
)

// SearchData 表示 SEARCH 命令返回的数据。
type SearchData struct {
	All NumSet // 所有结果

	// 需要 IMAP4rev2 或 ESEARCH
	UID   bool   // 是否返回 UID
	Min   uint32 // 最小值
	Max   uint32 // 最大值
	Count uint32 // 计数

	// 需要 CONDSTORE
	ModSeq uint64 // ModSeq 值
}

// AllSeqNums 方法返回 All 作为消息序号的切片。
//
// 返回：
// 消息序号的切片。
func (data *SearchData) AllSeqNums() []uint32 {
	seqSet, ok := data.All.(SeqSet)
	if !ok {
		return nil
	}

	// 注意：动态序号集将是服务器错误
	nums, ok := seqSet.Nums()
	if !ok {
		panic("imap: SearchData.All 是动态号码集")
	}
	return nums
}

// AllUIDs 方法返回 All 作为 UID 的切片。
//
// 返回：
// UID 的切片。
func (data *SearchData) AllUIDs() []UID {
	uidSet, ok := data.All.(UIDSet)
	if !ok {
		return nil
	}

	// 注意：动态序号集将是服务器错误
	uids, ok := uidSet.Nums()
	if !ok {
		panic("imap: SearchData.All 是动态号码集")
	}
	return uids
}

// searchRes 是一个特殊的空 UIDSet，用作标记。它具有非零容量，因此它的数据指针非 nil，可以用于比较。
//
// 它是 UIDSet 而非 SeqSet，因此它可以传递给 UID EXPUNGE 命令。
var (
	searchRes     = make(UIDSet, 0, 1)
	searchResAddr = reflect.ValueOf(searchRes).Pointer()
)

// SearchRes 方法返回一个特殊的标记，可以替代 UIDSet 引用上次 SEARCH 结果。在传输中，它被编码为 '$'。
//
// 需要 IMAP4rev2 或 SEARCHRES 扩展。
func SearchRes() UIDSet {
	return searchRes
}

// IsSearchRes 方法检查序号集是否引用了上次 SEARCH 结果。请参阅 SearchRes。
//
// 参数：
// - numSet: 要检查的序号集。
//
// 返回：
// 如果是上次搜索结果的引用，返回 true；否则返回 false。
func IsSearchRes(numSet NumSet) bool {
	return reflect.ValueOf(numSet).Pointer() == searchResAddr
}
