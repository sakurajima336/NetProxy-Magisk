package policy

import (
	"strings"
	"testing"
)

func TestParseListBasic(t *testing.T) {
	input := `
# OpenAI 规则组
domain_suffix:openai.com
domain_suffix:chatgpt.com
domain_keyword:openai
ip_cidr:23.102.140.0/22
process_name:com.openai.chatgpt
`
	result, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Count != 5 {
		t.Fatalf("条目数应为 5，实际 %d", result.Count)
	}
	ruleSet := result.ToRuleSet()
	if ruleSet["version"] != 1 {
		t.Fatalf("version 应为 1")
	}
	rules, ok := ruleSet["rules"].([]map[string]any)
	if !ok {
		t.Fatalf("rules 类型错误")
	}
	// domain_suffix 应合并成一条
	var suffixRule map[string]any
	for _, rule := range rules {
		if values, exists := rule["domain_suffix"]; exists {
			suffixRule = rule
			list, _ := values.([]any)
			if len(list) != 2 {
				t.Fatalf("domain_suffix 应有 2 个值，实际 %d", len(list))
			}
		}
	}
	if suffixRule == nil {
		t.Fatalf("未找到 domain_suffix 规则")
	}
}

func TestParseListDeduplicates(t *testing.T) {
	input := "domain_suffix:openai.com\ndomain_suffix:openai.com\n"
	result, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("重复项应被合并，实际 %d", result.Count)
	}
}

func TestParseListMultipleValues(t *testing.T) {
	input := "domain_suffix:openai.com, chatgpt.com\n"
	result, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Count != 2 {
		t.Fatalf("多值写法应产生 2 条，实际 %d", result.Count)
	}
}

func TestParseListRejectsUnknownField(t *testing.T) {
	_, err := ParseList(strings.NewReader("DOMAIN-SUFFIX:openai.com\n"))
	if err == nil {
		t.Fatal("应拒绝 Clash 风格字段名")
	}
	if !strings.Contains(err.Error(), "不支持的字段") {
		t.Fatalf("错误信息应说明字段不支持: %v", err)
	}
}

func TestParseListRejectsMissingColon(t *testing.T) {
	_, err := ParseList(strings.NewReader("openai.com\n"))
	if err == nil {
		t.Fatal("缺少冒号应报错")
	}
}

func TestParseListNumbersAndBool(t *testing.T) {
	input := "port:443\nport:8443\nip_is_private:true\n"
	result, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	ruleSet := result.ToRuleSet()
	rules := ruleSet["rules"].([]map[string]any)
	for _, rule := range rules {
		if values, exists := rule["port"]; exists {
			list := values.([]any)
			if len(list) != 2 {
				t.Fatalf("port 应有 2 个值")
			}
			if _, ok := list[0].(int); !ok {
				t.Fatalf("port 应为整数类型，实际 %T", list[0])
			}
		}
	}
}

func TestParseListEnumValidation(t *testing.T) {
	_, err := ParseList(strings.NewReader("network:sctp\n"))
	if err == nil {
		t.Fatal("network 只接受 tcp/udp/icmp")
	}
}

func TestParseListInlineComment(t *testing.T) {
	input := "domain_suffix:openai.com # 注释\n"
	result, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Count != 1 {
		t.Fatalf("行内注释应被剥离，实际 %d", result.Count)
	}
}

func TestSplitNodeRef(t *testing.T) {
	group, tag, ok := SplitNodeRef("abc123/赔钱🇯🇵日本东京06")
	if !ok || group != "abc123" || tag != "赔钱🇯🇵日本东京06" {
		t.Fatalf("解析节点引用失败: %q %q %v", group, tag, ok)
	}
	if _, _, ok := SplitNodeRef("no-slash"); ok {
		t.Fatal("缺少斜杠应失败")
	}
	if _, _, ok := SplitNodeRef("/only-tag"); ok {
		t.Fatal("缺少分组 ID 应失败")
	}
}

func TestConfigValidateRejectsDanglingRule(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules:   []RuleSet{{Name: "OpenAI", Group: "不存在的组"}},
	}
	err := config.Validate()
	if err == nil {
		t.Fatal("规则组引用了不存在的节点组应报错")
	}
	if !strings.Contains(err.Error(), "不存在的节点组") {
		t.Fatalf("错误信息不明确: %v", err)
	}
}

func TestConfigValidateRejectsDuplicateGroup(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}, {Tag: "AI"}},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("重复节点组名应报错")
	}
}

func TestConfigValidateRejectsBadRegexp(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI", Include: "([unclosed"}},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("非法正则应报错")
	}
}

func TestRemoveGroupRefusesWhenReferenced(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules:   []RuleSet{{Name: "OpenAI", Group: "AI"}},
	}
	if err := config.RemoveGroup("AI"); err == nil {
		t.Fatal("被引用的节点组不应允许删除")
	}
	if err := config.RemoveRule("OpenAI"); err != nil {
		t.Fatalf("删除规则组失败: %v", err)
	}
	if err := config.RemoveGroup("AI"); err != nil {
		t.Fatalf("解除引用后应可删除: %v", err)
	}
}

func TestUpsertKeepsSorted(t *testing.T) {
	config := Config{Version: 1}
	config.UpsertGroup(Group{Tag: "C"})
	config.UpsertGroup(Group{Tag: "A"})
	config.UpsertGroup(Group{Tag: "B"})
	if config.Groups[0].Tag != "A" || config.Groups[1].Tag != "B" || config.Groups[2].Tag != "C" {
		t.Fatalf("节点组应按名称排序: %+v", config.Groups)
	}
	// 重复 upsert 不应新增
	config.UpsertGroup(Group{Tag: "A", Strategy: "manual"})
	if len(config.Groups) != 3 {
		t.Fatalf("重复 upsert 不应新增，实际 %d", len(config.Groups))
	}
	if group, _ := config.FindGroup("A"); group.EffectiveStrategy() != "manual" {
		t.Fatalf("upsert 应更新已有节点组")
	}
}
