package policy

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// listFieldKind 描述一个 .list 字段的取值类型。
type listFieldKind int

const (
	// kindString 是字符串列表字段，如 domain_suffix。
	kindString listFieldKind = iota
	// kindNumber 是整数列表字段，如 port。
	kindNumber
	// kindBool 是布尔字段，如 ip_is_private。
	kindBool
	// kindStringSingle 是单值字符串字段，如 domain_match_strategy。
	kindStringSingle
)

// listField 是一个受支持的 .list 字段定义。
type listField struct {
	kind listFieldKind
	// enum 非空时限制取值集合。
	enum []string
}

// listFields 是 .list 允许使用的字段白名单，字段名与 sing-box rule-set 一致。
// 只收录在 headless rule（rule-set）中真正有意义的匹配项。
var listFields = map[string]listField{
	// 域名类
	"domain":         {kind: kindString},
	"domain_suffix":  {kind: kindString},
	"domain_keyword": {kind: kindString},
	"domain_regex":   {kind: kindString},
	"domain_match_strategy": {kind: kindStringSingle, enum: []string{
		"hybrid", "full",
	}},

	// IP 类
	"ip_cidr":              {kind: kindString},
	"source_ip_cidr":       {kind: kindString},
	"ip_is_private":        {kind: kindBool},
	"source_ip_is_private": {kind: kindBool},
	"ip_version":           {kind: kindNumber, enum: []string{"4", "6"}},

	// 端口类
	"port":              {kind: kindNumber},
	"port_range":        {kind: kindString},
	"source_port":       {kind: kindNumber},
	"source_port_range": {kind: kindString},

	// 进程 / 应用类
	"process_name":       {kind: kindString},
	"process_path":       {kind: kindString},
	"process_path_regex": {kind: kindString},
	"package_name":       {kind: kindString},
	"package_name_regex": {kind: kindString},

	// 协议 / 网络类
	"network": {kind: kindString, enum: []string{"tcp", "udp", "icmp"}},
	"protocol": {kind: kindString, enum: []string{
		"tls", "http", "quic", "dns", "stun", "bittorrent", "dtls", "ssh", "rdp", "ntp",
	}},
	"client":     {kind: kindString},
	"auth_user":  {kind: kindString},
	"user":       {kind: kindString},
	"user_id":    {kind: kindNumber},
	"clash_mode": {kind: kindString},
}

// listRule 是解析后的一条规则，字段顺序保留用于稳定输出。
type listRule struct {
	fields map[string]any
	order  []string
}

// ListParseResult 是 .list 的解析结果。
type ListParseResult struct {
	// Rules 是可用的 headless 规则，可直接序列化为 sing-box rule-set。
	Rules []listRule
	// Warnings 是非致命问题，例如重复项被合并。
	Warnings []string
	// Count 是有效条目数。
	Count int
}

// ParseListFile 解析一个 .list 文件。
func ParseListFile(path string) (ListParseResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return ListParseResult{}, err
	}
	defer file.Close()
	result, err := ParseList(file)
	if err != nil {
		return ListParseResult{}, fmt.Errorf("%s: %w", path, err)
	}
	return result, nil
}

// ParseList 解析 .list 内容。
//
// 语法与 sing-box rule-set 字段保持一致，一行一条：
//
//	# 注释
//	domain_suffix:openai.com
//	ip_cidr:1.2.3.0/24
//	port:443
//
// 也接受 `domain_suffix: openai.com` 这种带空格的写法，以及多值写法
// `domain_suffix:openai.com,chatgpt.com`。
func ParseList(reader io.Reader) (ListParseResult, error) {
	result := ListParseResult{Rules: []listRule{}}
	buckets := make(map[string]*listRule)
	seen := make(map[string]map[string]bool)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if index := strings.IndexByte(line, '#'); index >= 0 {
			line = line[:index]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		field, rawValue, found := strings.Cut(line, ":")
		if !found {
			return ListParseResult{}, fmt.Errorf("第 %d 行缺少冒号，应为 <字段>:<值>", lineNumber)
		}
		field = strings.TrimSpace(field)
		rawValue = strings.TrimSpace(rawValue)
		spec, supported := listFields[field]
		if !supported {
			return ListParseResult{}, fmt.Errorf("第 %d 行使用了不支持的字段 %q", lineNumber, field)
		}
		if rawValue == "" {
			return ListParseResult{}, fmt.Errorf("第 %d 行 %s 的值为空", lineNumber, field)
		}

		values := []string{rawValue}
		if spec.kind != kindStringSingle {
			values = splitListValues(rawValue)
		}
		if len(values) == 0 {
			return ListParseResult{}, fmt.Errorf("第 %d 行 %s 的值为空", lineNumber, field)
		}

		bucket, exists := buckets[field]
		if !exists {
			bucket = &listRule{fields: make(map[string]any)}
			buckets[field] = bucket
			seen[field] = make(map[string]bool)
		}

		for _, value := range values {
			converted, err := convertListValue(field, spec, value, lineNumber)
			if err != nil {
				return ListParseResult{}, err
			}
			key := fmt.Sprint(converted)
			if seen[field][key] {
				continue
			}
			seen[field][key] = true
			bucket.fields[field] = appendValue(bucket.fields[field], spec, converted)
			result.Count++
		}
	}
	if err := scanner.Err(); err != nil {
		return ListParseResult{}, err
	}

	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		bucket := buckets[name]
		bucket.order = append(bucket.order, name)
		result.Rules = append(result.Rules, *bucket)
	}
	return result, nil
}

func appendValue(existing any, spec listField, value any) any {
	switch spec.kind {
	case kindStringSingle:
		return value
	case kindBool:
		return value
	default:
		if existing == nil {
			return []any{value}
		}
		return append(existing.([]any), value)
	}
}

func convertListValue(field string, spec listField, value string, lineNumber int) (any, error) {
	switch spec.kind {
	case kindBool:
		switch strings.ToLower(value) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		default:
			return nil, fmt.Errorf("第 %d 行 %s 需要布尔值（true/false）", lineNumber, field)
		}
	case kindNumber:
		number, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 %s 需要整数: %s", lineNumber, field, value)
		}
		if len(spec.enum) > 0 && !containsString(spec.enum, value) {
			return nil, fmt.Errorf("第 %d 行 %s 只接受 %s", lineNumber, field, strings.Join(spec.enum, "/"))
		}
		return number, nil
	default:
		if len(spec.enum) > 0 && !containsString(spec.enum, value) {
			return nil, fmt.Errorf("第 %d 行 %s 只接受 %s", lineNumber, field, strings.Join(spec.enum, "/"))
		}
		return value, nil
	}
}

func splitListValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ToRuleSet 把解析结果转换为 sing-box rule-set 结构（可直接 JSON 序列化）。
func (r ListParseResult) ToRuleSet() map[string]any {
	rules := make([]map[string]any, 0, len(r.Rules))
	for _, rule := range r.Rules {
		rules = append(rules, rule.fields)
	}
	return map[string]any{
		"version": 1,
		"rules":   rules,
	}
}

// SupportedListFields 返回全部受支持的 .list 字段名，用于帮助与补全。
func SupportedListFields() []string {
	names := make([]string, 0, len(listFields))
	for name := range listFields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
