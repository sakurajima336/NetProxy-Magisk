package policy

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
)

// EntryKind 是规则条目的匹配方式。
//
// 面向"加一个域名/IP 就能用"的场景，只暴露三种最常用的语义，
// 而不是把 sing-box 的全部字段摊给用户。
type EntryKind string

const (
	// EntryDomain 匹配完整域名，等价 domain。
	EntryDomain EntryKind = "domain"
	// EntrySuffix 匹配域名及其子域名，等价 domain_suffix。
	EntrySuffix EntryKind = "suffix"
	// EntryKeyword 匹配域名中包含该关键字，等价 domain_keyword。
	EntryKeyword EntryKind = "keyword"
	// EntryIP 匹配 IP 或网段，等价 ip_cidr。
	EntryIP EntryKind = "ip"
)

// Entry 是一条规则条目，对应 .list 里的一行。
type Entry struct {
	// Kind 是匹配方式。
	Kind EntryKind `json:"kind"`
	// Value 是域名、关键字或 IP/CIDR。
	Value string `json:"value"`
}

// EntryKindLabel 返回面向用户的中文说明。
func EntryKindLabel(kind EntryKind) string {
	switch kind {
	case EntryDomain:
		return "只匹配这个域名"
	case EntrySuffix:
		return "包含这个域名就走（含子域名）"
	case EntryKeyword:
		return "域名里含这个关键字就走"
	case EntryIP:
		return "IP 或网段"
	default:
		return string(kind)
	}
}

// EntryKinds 返回全部匹配方式，顺序固定便于界面展示。
func EntryKinds() []EntryKind {
	return []EntryKind{EntrySuffix, EntryDomain, EntryKeyword, EntryIP}
}

// 域名做粗略校验：只拦明显写错的情况，不做严格 DNS 校验。
// IP 交给 net/netip 真正解析，不用正则。
var domainPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)

// ValidateEntry 校验一条条目，返回规范化后的值。
func ValidateEntry(kind EntryKind, rawValue string) (string, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return "", errors.New("内容不能为空")
	}
	// 用户常从浏览器地址栏复制，这里顺手去掉常见前缀与路径。
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimPrefix(value, "https://")
	if index := strings.IndexAny(value, "/?#"); index >= 0 && kind != EntryIP {
		value = value[:index]
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" {
		return "", errors.New("内容不能为空")
	}

	switch kind {
	case EntryDomain, EntrySuffix:
		if strings.ContainsAny(value, " :,") {
			return "", fmt.Errorf("%q 不是有效域名", value)
		}
		if !strings.Contains(value, ".") {
			return "", fmt.Errorf("%q 缺少后缀，例如 example.com", value)
		}
		if !domainPattern.MatchString(value) {
			return "", fmt.Errorf("%q 不是有效域名", value)
		}
		return strings.ToLower(value), nil
	case EntryKeyword:
		if strings.ContainsAny(value, " :,") {
			return "", fmt.Errorf("%q 不能包含空格或冒号", value)
		}
		return strings.ToLower(value), nil
	case EntryIP:
		// 用 netip 真正解析，避免 999.1.1.1 这类越界地址被正则放过。
		if prefix, err := netip.ParsePrefix(value); err == nil {
			return prefix.Masked().String(), nil
		}
		if address, err := netip.ParseAddr(value); err == nil {
			return address.String(), nil
		}
		return "", fmt.Errorf("%q 不是有效 IP 或网段，例如 1.2.3.4 或 1.2.3.0/24", value)
	default:
		return "", fmt.Errorf("未知匹配方式: %s", kind)
	}
}

// entryField 把条目类型映射到 sing-box 字段名。
func entryField(kind EntryKind) (string, bool) {
	switch kind {
	case EntryDomain:
		return "domain", true
	case EntrySuffix:
		return "domain_suffix", true
	case EntryKeyword:
		return "domain_keyword", true
	case EntryIP:
		return "ip_cidr", true
	default:
		return "", false
	}
}

// fieldEntryKind 是 entryField 的反向映射。
func fieldEntryKind(field string) (EntryKind, bool) {
	switch field {
	case "domain":
		return EntryDomain, true
	case "domain_suffix":
		return EntrySuffix, true
	case "domain_keyword":
		return EntryKeyword, true
	case "ip_cidr":
		return EntryIP, true
	default:
		return "", false
	}
}

// EntryList 是 .list 解析出的条目列表。
type EntryList struct {
	// Entries 是全部条目，按类型与值排序。
	Entries []Entry `json:"entries"`
	// Other 是无法归入上述四种类型的行数（用户手写的其他字段）。
	// 这些内容在重写文件时会被原样保留。
	Other int `json:"other"`
}

// ParseEntries 从 .list 内容解析出条目。
//
// 只识别四种面向用户的类型；其他合法字段（如 port、process_name）
// 计入 Other 并原样保留，不会被本功能覆盖。
func ParseEntries(reader interface{ Read([]byte) (int, error) }) (EntryList, error) {
	parsed, err := ParseList(reader)
	if err != nil {
		return EntryList{}, err
	}
	return entriesFromParse(parsed), nil
}

// ParseEntriesFile 解析指定 .list 文件的条目。
func ParseEntriesFile(path string) (EntryList, error) {
	parsed, err := ParseListFile(path)
	if err != nil {
		return EntryList{}, err
	}
	return entriesFromParse(parsed), nil
}

func entriesFromParse(parsed ListParseResult) EntryList {
	result := EntryList{Entries: []Entry{}}
	for _, rule := range parsed.Rules {
		for field, value := range rule.fields {
			kind, supported := fieldEntryKind(field)
			if !supported {
				// 统计其他字段涉及的条目数，提示用户存在手写内容。
				switch typed := value.(type) {
				case []any:
					result.Other += len(typed)
				default:
					result.Other++
				}
				continue
			}
			switch typed := value.(type) {
			case []any:
				for _, item := range typed {
					result.Entries = append(result.Entries, Entry{Kind: kind, Value: fmt.Sprint(item)})
				}
			default:
				result.Entries = append(result.Entries, Entry{Kind: kind, Value: fmt.Sprint(typed)})
			}
		}
	}
	sortEntries(result.Entries)
	return result
}

func sortEntries(entries []Entry) {
	order := map[EntryKind]int{}
	for index, kind := range EntryKinds() {
		order[kind] = index
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if order[entries[i].Kind] != order[entries[j].Kind] {
			return order[entries[i].Kind] < order[entries[j].Kind]
		}
		return entries[i].Value < entries[j].Value
	})
}

// SaveEntries 把条目写回 .list 文件。
//
// 用户手写的其他字段会被原样保留在文件末尾，避免本功能覆盖高级用法。
func SaveEntries(path string, entries []Entry) error {
	if err := os.MkdirAll(dirOf(path), 0o700); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	preserved := ""
	if err == nil {
		preserved = collectOtherLines(string(existing))
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	sorted := append([]Entry(nil), entries...)
	sortEntries(sorted)

	var builder strings.Builder
	builder.WriteString("# 一行一条，客户端可增删\n")
	for _, entry := range sorted {
		field, supported := entryField(entry.Kind)
		if !supported {
			return fmt.Errorf("未知匹配方式: %s", entry.Kind)
		}
		builder.WriteString(field)
		builder.WriteString(":")
		builder.WriteString(entry.Value)
		builder.WriteString("\n")
	}
	if preserved != "" {
		builder.WriteString("\n# 以下内容由其他方式维护，客户端不会改动\n")
		builder.WriteString(preserved)
	}
	return os.WriteFile(path, []byte(builder.String()), 0o600)
}

// collectOtherLines 提取非四种类型的行，供写回时保留。
func collectOtherLines(content string) string {
	var lines []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, _, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if _, supported := fieldEntryKind(strings.TrimSpace(field)); supported {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// AddEntries 追加条目，自动跳过重复项，返回实际新增数量。
func AddEntries(path string, additions []Entry) (int, EntryList, error) {
	current, err := ParseEntriesFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, EntryList{}, err
	}
	existing := make(map[string]bool, len(current.Entries))
	for _, entry := range current.Entries {
		existing[string(entry.Kind)+"\x00"+entry.Value] = true
	}
	added := 0
	for _, entry := range additions {
		key := string(entry.Kind) + "\x00" + entry.Value
		if existing[key] {
			continue
		}
		existing[key] = true
		current.Entries = append(current.Entries, entry)
		added++
	}
	if err := SaveEntries(path, current.Entries); err != nil {
		return 0, EntryList{}, err
	}
	updated, err := ParseEntriesFile(path)
	return added, updated, err
}

// RemoveEntries 按 (类型, 值) 删除条目，返回实际删除数量。
func RemoveEntries(path string, targets []Entry) (int, EntryList, error) {
	current, err := ParseEntriesFile(path)
	if err != nil {
		return 0, EntryList{}, err
	}
	remove := make(map[string]bool, len(targets))
	for _, entry := range targets {
		remove[string(entry.Kind)+"\x00"+entry.Value] = true
	}
	kept := make([]Entry, 0, len(current.Entries))
	removed := 0
	for _, entry := range current.Entries {
		if remove[string(entry.Kind)+"\x00"+entry.Value] {
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	if err := SaveEntries(path, kept); err != nil {
		return 0, EntryList{}, err
	}
	updated, err := ParseEntriesFile(path)
	return removed, updated, err
}

func dirOf(path string) string {
	index := strings.LastIndexByte(path, '/')
	if index < 0 {
		return "."
	}
	return path[:index]
}
