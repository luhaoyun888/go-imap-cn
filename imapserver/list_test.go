package imapserver_test

import (
	"testing"

	"github.com/luhaoyun888/go-imap-cn/imapserver"
)

// matchListTests 包含匹配测试的结构体数组。
// 每个结构体包含以下字段：
//
//	name - 邮箱名称
//	ref - 引用
//	pattern - 匹配模式
//	result - 预期的匹配结果
var matchListTests = []struct {
	name, ref, pattern string // 邮箱名称、引用和匹配模式
	result             bool   // 预期结果
}{
	{name: "INBOX", pattern: "INBOX", result: true},                                                    // 完全匹配
	{name: "INBOX", pattern: "Asuka", result: false},                                                   // 不匹配
	{name: "INBOX", pattern: "*", result: true},                                                        // 匹配任意内容
	{name: "INBOX", pattern: "%", result: true},                                                        // 匹配任意内容
	{name: "Neon Genesis Evangelion/Misato", pattern: "*", result: true},                               // 匹配任意内容
	{name: "Neon Genesis Evangelion/Misato", pattern: "%", result: false},                              // 不匹配
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neon Genesis Evangelion/*", result: true},       // 匹配子邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neon Genesis Evangelion/%", result: true},       // 匹配子邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neo* Evangelion/Misato", result: true},          // 匹配包含的邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neo% Evangelion/Misato", result: true},          // 匹配包含的邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "*Eva*/Misato", result: true},                    // 匹配包含的邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "%Eva%/Misato", result: true},                    // 匹配包含的邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "*X*/Misato", result: false},                     // 不匹配
	{name: "Neon Genesis Evangelion/Misato", pattern: "%X%/Misato", result: false},                     // 不匹配
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neon Genesis Evangelion/Mi%o", result: true},    // 匹配包含的邮箱
	{name: "Neon Genesis Evangelion/Misato", pattern: "Neon Genesis Evangelion/Mi%too", result: false}, // 不匹配
	{name: "Misato/Misato", pattern: "Mis*to/Misato", result: true},                                    // 匹配包含的邮箱
	{name: "Misato/Misato", pattern: "Mis*to", result: true},                                           // 匹配包含的邮箱
	{name: "Misato/Misato/Misato", pattern: "Mis*to/Mis%to", result: true},                             // 匹配包含的邮箱
	{name: "Misato/Misato", pattern: "Mis**to/Misato", result: true},                                   // 匹配包含的邮箱
	{name: "Misato/Misato", pattern: "Misat%/Misato", result: true},                                    // 匹配包含的邮箱
	{name: "Misato/Misato", pattern: "Misat%Misato", result: false},                                    // 不匹配
	{name: "Misato/Misato", ref: "Misato", pattern: "Misato", result: true},                            // 完全匹配
	{name: "Misato/Misato", ref: "Misato/", pattern: "Misato", result: true},                           // 完全匹配
	{name: "Misato/Misato", ref: "Shinji", pattern: "/Misato/*", result: true},                         // 匹配子邮箱
	{name: "Misato/Misato", ref: "Misato", pattern: "/Misato", result: false},                          // 不匹配
	{name: "Misato/Misato", ref: "Misato", pattern: "Shinji", result: false},                           // 不匹配
	{name: "Misato/Misato", ref: "Shinji", pattern: "Misato", result: false},                           // 不匹配
}

// TestMatchList 测试 MatchList 函数。
func TestMatchList(t *testing.T) {
	delim := '/' // 分隔符
	for _, test := range matchListTests {
		result := imapserver.MatchList(test.name, delim, test.ref, test.pattern)
		if result != test.result {
			t.Errorf("匹配名称 %q 和模式 %q 及引用 %q 返回 %v，预期 %v", test.name, test.pattern, test.ref, result, test.result) // 测试失败信息
		}
	}
}
