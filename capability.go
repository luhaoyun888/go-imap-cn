package imap

import (
	"strconv"
	"strings"
)

// Cap 表示 IMAP 的能力。
type Cap string

// 注册的能力。
//
// 参见：https://www.iana.org/assignments/imap-capabilities/
const (
	CapIMAP4rev1 Cap = "IMAP4rev1" // RFC 3501
	CapIMAP4rev2 Cap = "IMAP4rev2" // RFC 9051

	CapAuthPlain Cap = "AUTH=PLAIN"

	CapStartTLS      Cap = "STARTTLS"      // 支持 STARTTLS
	CapLoginDisabled Cap = "LOGINDISABLED" // 登录被禁用

	// 在 IMAP4rev2 中折叠
	CapNamespace    Cap = "NAMESPACE"     // 支持 NAMESPACE，RFC 2342
	CapUnselect     Cap = "UNSELECT"      // 支持 UNSELECT，RFC 3691
	CapUIDPlus      Cap = "UIDPLUS"       // 支持 UIDPLUS，RFC 4315
	CapESearch      Cap = "ESEARCH"       // 支持 ESEARCH，RFC 4731
	CapSearchRes    Cap = "SEARCHRES"     // 支持 SEARCHRES，RFC 5182
	CapEnable       Cap = "ENABLE"        // 支持 ENABLE，RFC 5161
	CapIdle         Cap = "IDLE"          // 支持 IDLE，RFC 2177
	CapSASLIR       Cap = "SASL-IR"       // 支持 SASL-IR，RFC 4959
	CapListExtended Cap = "LIST-EXTENDED" // 支持 LIST-EXTENDED，RFC 5258
	CapListStatus   Cap = "LIST-STATUS"   // 支持 LIST-STATUS，RFC 5819
	CapMove         Cap = "MOVE"          // 支持 MOVE，RFC 6851
	CapLiteralMinus Cap = "LITERAL-"      // 支持 LITERAL-，RFC 7888
	CapStatusSize   Cap = "STATUS=SIZE"   // 支持 STATUS=SIZE，RFC 8438

	CapACL              Cap = "ACL"                // 支持 ACL，RFC 4314
	CapAppendLimit      Cap = "APPENDLIMIT"        // 支持 APPENDLIMIT，RFC 7889
	CapBinary           Cap = "BINARY"             // 支持 BINARY，RFC 3516
	CapCatenate         Cap = "CATENATE"           // 支持 CATENATE，RFC 4469
	CapChildren         Cap = "CHILDREN"           // 支持 CHILDREN，RFC 3348
	CapCondStore        Cap = "CONDSTORE"          // 支持 CONDSTORE，RFC 7162
	CapConvert          Cap = "CONVERT"            // 支持 CONVERT，RFC 5259
	CapCreateSpecialUse Cap = "CREATE-SPECIAL-USE" // 支持 CREATE-SPECIAL-USE，RFC 6154
	CapESort            Cap = "ESORT"              // 支持 ESORT，RFC 5267
	CapFilters          Cap = "FILTERS"            // 支持 FILTERS，RFC 5466
	CapID               Cap = "ID"                 // 支持 ID，RFC 2971
	CapLanguage         Cap = "LANGUAGE"           // 支持 LANGUAGE，RFC 5255
	CapListMyRights     Cap = "LIST-MYRIGHTS"      // 支持 LIST-MYRIGHTS，RFC 8440
	CapLiteralPlus      Cap = "LITERAL+"           // 支持 LITERAL+，RFC 7888
	CapLoginReferrals   Cap = "LOGIN-REFERRALS"    // 支持 LOGIN-REFERRALS，RFC 2221
	CapMailboxReferrals Cap = "MAILBOX-REFERRALS"  // 支持 MAILBOX-REFERRALS，RFC 2193
	CapMetadata         Cap = "METADATA"           // 支持 METADATA，RFC 5464
	CapMetadataServer   Cap = "METADATA-SERVER"    // 支持 METADATA-SERVER，RFC 5464
	CapMultiAppend      Cap = "MULTIAPPEND"        // 支持 MULTIAPPEND，RFC 3502
	CapMultiSearch      Cap = "MULTISEARCH"        // 支持 MULTISEARCH，RFC 7377
	CapNotify           Cap = "NOTIFY"             // 支持 NOTIFY，RFC 5465
	CapObjectID         Cap = "OBJECTID"           // 支持 OBJECTID，RFC 8474
	CapPreview          Cap = "PREVIEW"            // 支持 PREVIEW，RFC 8970
	CapQResync          Cap = "QRESYNC"            // 支持 QRESYNC，RFC 7162
	CapQuota            Cap = "QUOTA"              // 支持 QUOTA，RFC 9208
	CapQuotaSet         Cap = "QUOTASET"           // 支持 QUOTASET，RFC 9208
	CapReplace          Cap = "REPLACE"            // 支持 REPLACE，RFC 8508
	CapSaveDate         Cap = "SAVEDATE"           // 支持 SAVEDATE，RFC 8514
	CapSearchFuzzy      Cap = "SEARCH=FUZZY"       // 支持 SEARCH=FUZZY，RFC 6203
	CapSort             Cap = "SORT"               // 支持 SORT，RFC 5256
	CapSortDisplay      Cap = "SORT=DISPLAY"       // 支持 SORT=DISPLAY，RFC 5957
	CapSpecialUse       Cap = "SPECIAL-USE"        // 支持 SPECIAL-USE，RFC 6154
	CapUnauthenticate   Cap = "UNAUTHENTICATE"     // 支持 UNAUTHENTICATE，RFC 8437
	CapURLPartial       Cap = "URL-PARTIAL"        // 支持 URL-PARTIAL，RFC 5550
	CapURLAuth          Cap = "URLAUTH"            // 支持 URLAUTH，RFC 4467
	CapUTF8Accept       Cap = "UTF8=ACCEPT"        // 支持 UTF8=ACCEPT，RFC 6855
	CapUTF8Only         Cap = "UTF8=ONLY"          // 支持 UTF8=ONLY，RFC 6855
	CapWithin           Cap = "WITHIN"             // 支持 WITHIN，RFC 5032
	CapUIDOnly          Cap = "UIDONLY"            // 支持 UIDONLY，RFC 9586
	CapListMetadata     Cap = "LIST-METADATA"      // 支持 LIST-METADATA，RFC 9590
	CapInProgress       Cap = "INPROGRESS"         // 支持 INPROGRESS，RFC 9585
)

// imap4rev2Caps 是 IMAP4rev2 的能力集合。
var imap4rev2Caps = CapSet{
	CapNamespace:    {},
	CapUnselect:     {},
	CapUIDPlus:      {},
	CapESearch:      {},
	CapSearchRes:    {},
	CapEnable:       {},
	CapIdle:         {},
	CapSASLIR:       {},
	CapListExtended: {},
	CapListStatus:   {},
	CapMove:         {},
	CapLiteralMinus: {},
	CapStatusSize:   {},
}

// AuthCap 返回 SASL 身份验证机制的能力名称。
func AuthCap(mechanism string) Cap {
	return Cap("AUTH=" + mechanism)
}

// CapSet 是能力集合的类型。
type CapSet map[Cap]struct{}

// has 检查能力集合中是否包含某个能力。
func (set CapSet) has(c Cap) bool {
	_, ok := set[c]
	return ok
}

// Has 检查能力集合是否支持某个能力。
//
// 一些能力由其他能力隐含，因此即使该能力不在集合中，Has 也可能返回 true。
func (set CapSet) Has(c Cap) bool {
	if set.has(c) {
		return true
	}

	if set.has(CapIMAP4rev2) && imap4rev2Caps.has(c) {
		return true
	}

	if c == CapLiteralMinus && set.has(CapLiteralPlus) {
		return true
	}
	if c == CapCondStore && set.has(CapQResync) {
		return true
	}
	if c == CapUTF8Accept && set.has(CapUTF8Only) {
		return true
	}
	if c == CapAppendLimit {
		_, ok := set.AppendLimit()
		return ok
	}

	return false
}

// AuthMechanisms 返回支持的 SASL 身份验证机制的列表。
func (set CapSet) AuthMechanisms() []string {
	var l []string
	for c := range set {
		if !strings.HasPrefix(string(c), "AUTH=") {
			continue
		}
		mech := strings.TrimPrefix(string(c), "AUTH=")
		l = append(l, mech)
	}
	return l
}

// AppendLimit 检查 APPENDLIMIT 能力。
//
// 如果服务器支持 APPENDLIMIT，则 ok 为 true。如果服务器没有对所有邮箱的相同上传限制，则 limit 为 nil，
// 每个邮箱的限制必须通过 STATUS 查询。
func (set CapSet) AppendLimit() (limit *uint32, ok bool) {
	if set.has(CapAppendLimit) {
		return nil, true
	}

	for c := range set {
		if !strings.HasPrefix(string(c), "APPENDLIMIT=") {
			continue
		}

		limitStr := strings.TrimPrefix(string(c), "APPENDLIMIT=")
		limit64, err := strconv.ParseUint(limitStr, 10, 32)
		if err == nil && limit64 > 0 {
			limit32 := uint32(limit64)
			return &limit32, true
		}
	}

	limit32 := ^uint32(0) // 返回 uint32 的最大值
	return &limit32, false
}

// QuotaResourceTypes 返回支持的 QUOTA 资源类型的列表。
func (set CapSet) QuotaResourceTypes() []QuotaResourceType {
	var l []QuotaResourceType
	for c := range set {
		if !strings.HasPrefix(string(c), "QUOTA=RES-") {
			continue
		}
		t := strings.TrimPrefix(string(c), "QUOTA=RES-")
		l = append(l, QuotaResourceType(t))
	}
	return l
}

// ThreadAlgorithms 返回支持的线程算法的列表。
func (set CapSet) ThreadAlgorithms() []ThreadAlgorithm {
	var l []ThreadAlgorithm
	for c := range set {
		if !strings.HasPrefix(string(c), "THREAD=") {
			continue
		}
		alg := strings.TrimPrefix(string(c), "THREAD=")
		l = append(l, ThreadAlgorithm(alg))
	}
	return l
}
