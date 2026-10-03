// Package policy 管理 NetProxy 的规则组与节点组。
//
// 用户侧只维护两类简单文件：
//
//	rules/*.list   规则组，sing-box 字段语法，一行一条
//	groups.json    节点组，记录每个规则组使用哪些订阅与节点
//
// 本包负责把它们编译成 sing-box 运行时片段（rule_set / outbounds / route.rules），
// 由 module.Prepare 输出到 runtime/policy.json，再通过额外的 -c 参数交给 sing-box。
// 这样主配置 config.json 始终不被修改。
package policy

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ListFileName 是节点组配置的固定文件名，位于 sing-box 配置根目录。
const ListFileName = "groups.json"

// DefaultOutbound 是节点组未配置任何来源时的兜底出站。
const DefaultOutbound = "Proxy"

// DefaultStrategy 是节点组默认的测速策略。
const DefaultStrategy = "urltest"

var (
	validName    = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._ -]*$`)
	validGroupID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Group 是一个节点组：规则组可以用哪些订阅与哪些单独节点。
type Group struct {
	// Tag 是节点组名，也是生成出来的出站 tag，规则组通过它引用。
	Tag string `json:"tag"`
	// Providers 是整份订阅的 Catalog 分组 ID，该订阅的全部节点都会纳入。
	Providers []string `json:"providers,omitempty"`
	// Nodes 是单独挑选的节点，格式为 <分组 ID>/<节点标签>。
	Nodes []string `json:"nodes,omitempty"`
	// Include 是正则筛选，对 Providers 与 Nodes 的结果再做一次过滤。
	Include string `json:"include,omitempty"`
	// Exclude 是正则排除，优先级高于 Include。
	Exclude string `json:"exclude,omitempty"`
	// Strategy 是测速方式：urltest（自动延迟最低）或 manual（手动选择）。
	Strategy string `json:"strategy,omitempty"`
	// Default 是 manual 策略下的默认选中项，留空表示第一个。
	Default string `json:"default,omitempty"`
}

// RuleSet 是一个规则组：一组规则，指定走哪个节点组。
type RuleSet struct {
	// Name 是规则组名，对应 rules/<Name>.list。
	Name string `json:"name"`
	// Group 是该规则组使用的节点组 tag。
	Group string `json:"group"`
	// Enabled 为 false 时跳过生成，便于临时停用。
	Enabled *bool `json:"enabled,omitempty"`
}

// Config 是 groups.json 的根结构。
type Config struct {
	// Version 是配置版本，当前为 1。
	Version int `json:"version"`
	// Groups 是节点组列表。
	Groups []Group `json:"groups,omitempty"`
	// Rules 是规则组列表。
	Rules []RuleSet `json:"rules,omitempty"`
}

// IsEnabled 返回规则组是否启用，缺省视为启用。
func (r RuleSet) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// EffectiveStrategy 返回规范化后的测速策略。
func (g Group) EffectiveStrategy() string {
	if g.Strategy == "" {
		return DefaultStrategy
	}
	return g.Strategy
}

// Load 读取 groups.json，文件不存在时返回空配置。
func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{Version: 1}, nil
		}
		return Config{}, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return Config{Version: 1}, nil
	}
	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return Config{}, fmt.Errorf("解析 %s 失败: %w", filepath.Base(path), err)
	}
	if config.Version == 0 {
		config.Version = 1
	}
	return config, nil
}

// Save 原子写入 groups.json。
func Save(path string, config Config) error {
	if config.Version == 0 {
		config.Version = 1
	}
	if err := config.Validate(); err != nil {
		return err
	}
	content, err := json.Marshal(config, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return writeAtomic(path, content)
}

// Validate 校验配置的完整性与唯一性。
func (c Config) Validate() error {
	seenGroup := make(map[string]bool, len(c.Groups))
	for index, group := range c.Groups {
		if strings.TrimSpace(group.Tag) == "" {
			return fmt.Errorf("groups[%d]: 节点组名不能为空", index)
		}
		if !validName.MatchString(group.Tag) {
			return fmt.Errorf("节点组名 %q 含非法字符", group.Tag)
		}
		if seenGroup[group.Tag] {
			return fmt.Errorf("节点组名重复: %s", group.Tag)
		}
		seenGroup[group.Tag] = true
		if strategy := group.EffectiveStrategy(); strategy != "urltest" && strategy != "manual" {
			return fmt.Errorf("节点组 %s: 未知测速策略 %q（可用 urltest / manual）", group.Tag, strategy)
		}
		if err := validateRegexp("include", group.Tag, group.Include); err != nil {
			return err
		}
		if err := validateRegexp("exclude", group.Tag, group.Exclude); err != nil {
			return err
		}
		for _, provider := range group.Providers {
			if !validGroupID.MatchString(provider) {
				return fmt.Errorf("节点组 %s: 非法订阅分组 ID %q", group.Tag, provider)
			}
		}
		for _, node := range group.Nodes {
			if _, _, ok := SplitNodeRef(node); !ok {
				return fmt.Errorf("节点组 %s: 节点引用 %q 应为 <分组 ID>/<节点标签>", group.Tag, node)
			}
		}
	}
	seenRule := make(map[string]bool, len(c.Rules))
	for index, rule := range c.Rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("rules[%d]: 规则组名不能为空", index)
		}
		if strings.ContainsAny(rule.Name, `/\`) || strings.Contains(rule.Name, "..") {
			return fmt.Errorf("规则组名 %q 含非法字符", rule.Name)
		}
		if seenRule[rule.Name] {
			return fmt.Errorf("规则组名重复: %s", rule.Name)
		}
		seenRule[rule.Name] = true
		if strings.TrimSpace(rule.Group) == "" {
			return fmt.Errorf("规则组 %s: 必须指定节点组", rule.Name)
		}
		if !seenGroup[rule.Group] {
			return fmt.Errorf("规则组 %s: 引用了不存在的节点组 %q", rule.Name, rule.Group)
		}
	}
	return nil
}

func validateRegexp(field, tag, value string) error {
	if value == "" {
		return nil
	}
	if _, err := regexp.Compile(value); err != nil {
		return fmt.Errorf("节点组 %s: %s 正则无效: %w", tag, field, err)
	}
	return nil
}

// SplitNodeRef 拆分 <分组 ID>/<节点标签>，节点标签允许包含斜杠。
func SplitNodeRef(reference string) (string, string, bool) {
	group, tag, found := strings.Cut(strings.TrimSpace(reference), "/")
	if !found || group == "" || tag == "" {
		return "", "", false
	}
	return group, tag, true
}

// FindGroup 按 tag 查找节点组。
func (c Config) FindGroup(tag string) (Group, bool) {
	for _, group := range c.Groups {
		if group.Tag == tag {
			return group, true
		}
	}
	return Group{}, false
}

// FindRule 按名称查找规则组。
func (c Config) FindRule(name string) (RuleSet, bool) {
	for _, rule := range c.Rules {
		if rule.Name == name {
			return rule, true
		}
	}
	return RuleSet{}, false
}

// UpsertGroup 新增或替换节点组，返回是否为新增。
func (c *Config) UpsertGroup(group Group) bool {
	for index, existing := range c.Groups {
		if existing.Tag == group.Tag {
			c.Groups[index] = group
			return false
		}
	}
	c.Groups = append(c.Groups, group)
	sort.Slice(c.Groups, func(i, j int) bool { return c.Groups[i].Tag < c.Groups[j].Tag })
	return true
}

// UpsertRule 新增或替换规则组，返回是否为新增。
func (c *Config) UpsertRule(rule RuleSet) bool {
	for index, existing := range c.Rules {
		if existing.Name == rule.Name {
			c.Rules[index] = rule
			return false
		}
	}
	c.Rules = append(c.Rules, rule)
	sort.Slice(c.Rules, func(i, j int) bool { return c.Rules[i].Name < c.Rules[j].Name })
	return true
}

// RemoveGroup 删除节点组，若仍被规则组引用则拒绝。
func (c *Config) RemoveGroup(tag string) error {
	for _, rule := range c.Rules {
		if rule.Group == tag {
			return fmt.Errorf("节点组 %s 仍被规则组 %s 引用，请先修改该规则组", tag, rule.Name)
		}
	}
	for index, existing := range c.Groups {
		if existing.Tag == tag {
			c.Groups = append(c.Groups[:index], c.Groups[index+1:]...)
			return nil
		}
	}
	return fmt.Errorf("节点组不存在: %s", tag)
}

// RemoveRule 删除规则组。
func (c *Config) RemoveRule(name string) error {
	for index, existing := range c.Rules {
		if existing.Name == name {
			c.Rules = append(c.Rules[:index], c.Rules[index+1:]...)
			return nil
		}
	}
	return fmt.Errorf("规则组不存在: %s", name)
}
