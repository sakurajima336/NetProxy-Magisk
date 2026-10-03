package policy

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/provider"
)

// RuleSetDirName 是生成的规则集目录名，位于 sing-box 配置根目录下。
// 与用户手写的 rules/local/ 分开，避免污染配置编辑器的可编辑文档列表。
const RuleSetDirName = "policy"

// FragmentFileName 是交给 sing-box 的运行时片段文件名。
const FragmentFileName = "policy.json"

// PickedProviderPrefix 是"单独挑选的节点"生成的 Provider tag 前缀。
//
// sing-box 在 StartStatePostStart 阶段先启动 outbound 再启动 provider，
// 因此 selector 的 outbounds 里直接引用 provider 下的节点会报
// "dependency not found"。把单独节点做成独立 Provider，再让 selector 通过
// providers 引用，可以绕开这个启动顺序限制。
const PickedProviderPrefix = "Picked/"

// PickedProviderTag 返回某个节点组"单独节点"Provider 的 tag。
func PickedProviderTag(groupTag string) string {
	return PickedProviderPrefix + groupTag
}

// CompileOptions 描述一次编译所需的路径与解析器。
type CompileOptions struct {
	// SingBoxDir 是 sing-box 静态配置根目录。
	SingBoxDir string
	// RuntimeDir 是运行时生成目录。
	RuntimeDir string
	// CatalogRoot 是 Catalog 根目录，用于把分组 ID 解析成运行时 Provider tag。
	CatalogRoot string
	// ProviderTag 把 Catalog 分组 ID 解析成运行时 Provider tag。
	// 为空时使用分组 ID 本身。
	ProviderTag func(ctx context.Context, groupID string) (string, error)
	// ProviderPath 把运行时 Provider tag 解析成其节点文件路径。
	// 用于把"单独挑选的节点"抽取成独立 Provider。
	ProviderPath func(ctx context.Context, providerTag string) (string, error)
}

// CompileResult 是编译产物。
type CompileResult struct {
	// FragmentPath 是生成的运行时片段路径。
	FragmentPath string `json:"fragment_path"`
	// RuleSetDir 是生成的规则集目录。
	RuleSetDir string `json:"rule_set_dir"`
	// GroupCount 是节点组数量。
	GroupCount int `json:"group_count"`
	// RuleSetCount 是规则组数量。
	RuleSetCount int `json:"rule_set_count"`
	// EntryCount 是规则条目总数。
	EntryCount int `json:"entry_count"`
	// MissingProviders 是引用了但不存在的 Catalog 分组。
	MissingProviders []string `json:"missing_providers,omitempty"`
	// MissingNodes 是无法解析成运行时 tag 的节点引用。
	MissingNodes []string `json:"missing_nodes,omitempty"`
	// MissingLists 是声明了但缺少 .list 文件的规则组。
	MissingLists []string `json:"missing_lists,omitempty"`
	// Warnings 是编译过程中的提示。
	Warnings []string `json:"warnings,omitempty"`
}

// RuleSetDir 返回生成的规则集目录。
func RuleSetDir(singBoxDir string) string {
	return filepath.Join(singBoxDir, "rules", RuleSetDirName)
}

// ListDir 返回用户手写 .list 的目录。
func ListDir(singBoxDir string) string {
	return filepath.Join(singBoxDir, "rules", "local")
}

// ConfigPath 返回 groups.json 路径。
func ConfigPath(singBoxDir string) string {
	return filepath.Join(singBoxDir, ListFileName)
}

// ListPath 返回某个规则组对应的 .list 路径。
func ListPath(singBoxDir, name string) string {
	return filepath.Join(ListDir(singBoxDir), name+".list")
}

// FragmentPath 返回运行时片段路径。
func FragmentPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, FragmentFileName)
}

// fragment 是交给 sing-box 的配置片段结构。
type fragment struct {
	Route     fragmentRoute      `json:"route"`
	Outbounds []map[string]any   `json:"outbounds"`
	Providers []fragmentProvider `json:"providers,omitempty"`
}

// fragmentProvider 承载"单独挑选的节点"，让 selector 能通过 providers 引用。
type fragmentProvider struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
	Path string `json:"path"`
}

type fragmentRoute struct {
	RuleSet []fragmentRuleSet `json:"rule_set"`
	Rules   []fragmentRule    `json:"rules"`
}

type fragmentRuleSet struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Format string `json:"format"`
	Path   string `json:"path"`
}

type fragmentRule struct {
	RuleSet  string `json:"rule_set"`
	Action   string `json:"action"`
	Outbound string `json:"outbound"`
}

// writePickedProvider 把节点组中"单独挑选的节点"抽取成一个独立 Provider 文件。
//
// nodeRefs 是已解析为 <Provider tag>/<节点标签> 的引用。同一个源 Provider 的
// 节点会合并进一个文件，保证 selector 能通过 providers 引用到它们。
func writePickedProvider(ctx context.Context, options CompileOptions, group Group, nodeRefs []string) (string, error) {
	byProvider := make(map[string][]string)
	for _, reference := range nodeRefs {
		providerTag, tag, ok := SplitNodeRef(reference)
		if !ok {
			continue
		}
		byProvider[providerTag] = append(byProvider[providerTag], tag)
	}
	sources := make([]string, 0, len(byProvider))
	for providerTag := range byProvider {
		sources = append(sources, providerTag)
	}
	sort.Strings(sources)

	outbounds := make([]any, 0, len(nodeRefs))
	for _, providerTag := range sources {
		if options.ProviderPath == nil {
			return "", fmt.Errorf("未配置 ProviderPath，无法解析订阅 %s", providerTag)
		}
		path, err := options.ProviderPath(ctx, providerTag)
		if err != nil {
			return "", fmt.Errorf("读取订阅 %s 失败: %w", providerTag, err)
		}
		document, err := provider.Load(ctx, path)
		if err != nil {
			return "", fmt.Errorf("读取订阅 %s 节点失败: %w", providerTag, err)
		}
		selected, err := provider.Filter(document, buildExactTagPattern(byProvider[providerTag]), "")
		if err != nil {
			return "", err
		}
		content, err := provider.Marshal(ctx, selected)
		if err != nil {
			return "", err
		}
		var document2 struct {
			Outbounds []any `json:"outbounds"`
		}
		if err := json.Unmarshal(content, &document2); err != nil {
			return "", err
		}
		outbounds = append(outbounds, document2.Outbounds...)
	}

	outputPath := filepath.Join(RuleSetDir(options.SingBoxDir), "picked-"+sanitizeFileName(group.Tag)+".json")
	content, err := json.Marshal(map[string]any{"outbounds": outbounds}, jsontext.WithIndent("  "))
	if err != nil {
		return "", err
	}
	content = append(content, '\n')
	if err := provider.WriteAtomic(outputPath, content, 0o600); err != nil {
		return "", err
	}
	return outputPath, nil
}

// buildExactTagPattern 构造只匹配指定节点标签的正则。
func buildExactTagPattern(tags []string) string {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, regexp.QuoteMeta(tag))
	}
	return "^(?:" + strings.Join(parts, "|") + ")$"
}

// sanitizeFileName 把节点组名转换为安全文件名。
func sanitizeFileName(name string) string {
	var builder strings.Builder
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "group"
	}
	return builder.String()
}

// buildOutbound 构造一个 selector / urltest 出站。
//
// 使用 map 而非结构体，因为 selector 与 urltest 的字段集不同，
// 且 json/v2 的 omitempty 不会省略数值 0，容易把 tolerance 带给 selector
// 导致 sing-box 报 unknown field。
//
// providerTags 是已解析为运行时 tag 的 Provider 列表，包含用户的订阅
// 以及由单独节点生成的 Picked/<节点组>。
func buildOutbound(group Group, providerTags []string) map[string]any {
	outbound := map[string]any{
		"tag":                         group.Tag,
		"interrupt_exist_connections": true,
	}
	if len(providerTags) == 0 {
		// 没有任何来源时指向兜底出站，避免空组导致启动失败。
		outbound["outbounds"] = []string{DefaultOutbound}
	} else {
		outbound["providers"] = providerTags
	}
	if group.Include != "" {
		outbound["include"] = group.Include
	}
	if group.Exclude != "" {
		outbound["exclude"] = group.Exclude
	}
	if group.EffectiveStrategy() == "manual" {
		outbound["type"] = "selector"
		if group.Default != "" {
			outbound["default"] = group.Default
		}
		return outbound
	}
	outbound["type"] = "urltest"
	outbound["url"] = "https://www.gstatic.com/generate_204"
	outbound["interval"] = "3m"
	outbound["tolerance"] = 50
	return outbound
}

// resolveNodeRefs 把 Catalog 格式的节点引用转换为运行时出站 tag。
//
// 用户写的是 <Catalog 分组 ID>/<节点标签>，而 sing-box 运行时里
// Provider 下的节点 tag 是 <Provider tag>/<节点标签>，两者必须转换，
// 否则 sing-box 会报 dependency not found。
func resolveNodeRefs(ctx context.Context, references []string, options CompileOptions) ([]string, []string) {
	resolved := make([]string, 0, len(references))
	var missing []string
	for _, reference := range references {
		groupID, tag, ok := SplitNodeRef(reference)
		if !ok {
			missing = append(missing, reference)
			continue
		}
		if options.ProviderTag == nil {
			resolved = append(resolved, reference)
			continue
		}
		providerTag, err := options.ProviderTag(ctx, groupID)
		if err != nil {
			missing = append(missing, reference)
			continue
		}
		resolved = append(resolved, providerTag+"/"+tag)
	}
	return resolved, missing
}

// Compile 把 groups.json 与 rules/*.list 编译成 sing-box 运行时片段。
//
// 生成内容：
//   - <singBoxDir>/rules/policy/<规则组>.json   每个规则组一个 rule-set
//   - <runtimeDir>/policy.json                  节点组出站 + rule_set 声明 + 路由规则
//
// 不修改主配置 config.json。
func Compile(ctx context.Context, config Config, options CompileOptions) (CompileResult, error) {
	if strings.TrimSpace(options.SingBoxDir) == "" {
		return CompileResult{}, errors.New("sing-box 配置目录不能为空")
	}
	if strings.TrimSpace(options.RuntimeDir) == "" {
		return CompileResult{}, errors.New("运行时目录不能为空")
	}
	if err := config.Validate(); err != nil {
		return CompileResult{}, err
	}

	result := CompileResult{
		FragmentPath: FragmentPath(options.RuntimeDir),
		RuleSetDir:   RuleSetDir(options.SingBoxDir),
	}

	ruleSetDir := RuleSetDir(options.SingBoxDir)
	if err := os.MkdirAll(ruleSetDir, 0o700); err != nil {
		return CompileResult{}, err
	}

	// 1. 节点组 -> selector / urltest 出站
	outbounds := make([]map[string]any, 0, len(config.Groups))
	providers := make([]fragmentProvider, 0, len(config.Groups))
	for _, group := range config.Groups {
		providerTags, missingProviders := resolveProviderTags(ctx, group.Providers, options)
		for _, id := range missingProviders {
			result.MissingProviders = append(result.MissingProviders, fmt.Sprintf("%s -> %s", group.Tag, id))
		}

		// 单独挑选的节点不能直接放进 selector.outbounds：
		// sing-box 先启动 outbound 再启动 provider，直接引用会 dependency not found。
		// 因此把它们写成独立 Provider，再让 selector 通过 providers 引用。
		nodeRefs, missingNodes := resolveNodeRefs(ctx, group.Nodes, options)
		for _, reference := range missingNodes {
			result.MissingNodes = append(result.MissingNodes, fmt.Sprintf("%s -> %s", group.Tag, reference))
		}
		if len(nodeRefs) > 0 {
			pickedPath, err := writePickedProvider(ctx, options, group, nodeRefs)
			if err != nil {
				return CompileResult{}, err
			}
			pickedTag := PickedProviderTag(group.Tag)
			providers = append(providers, fragmentProvider{Type: "local", Tag: pickedTag, Path: pickedPath})
			providerTags = append(providerTags, pickedTag)
		}

		if len(providerTags) == 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("节点组 %s 没有可用来源，已回退到 %s", group.Tag, DefaultOutbound))
		}
		// default 同样需要从 Catalog 引用转换为运行时 tag。
		resolved := group
		if resolved.Default != "" {
			defaults, _ := resolveNodeRefs(ctx, []string{resolved.Default}, options)
			if len(defaults) == 1 {
				resolved.Default = defaults[0]
			}
		}
		outbounds = append(outbounds, buildOutbound(resolved, providerTags))
	}

	// 2. 规则组 -> rule-set 文件 + 路由规则
	ruleSets := make([]fragmentRuleSet, 0, len(config.Rules))
	rules := make([]fragmentRule, 0, len(config.Rules))
	for _, rule := range config.Rules {
		if !rule.IsEnabled() {
			continue
		}
		listPath := ListPath(options.SingBoxDir, rule.Name)
		parsed, err := ParseListFile(listPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				result.MissingLists = append(result.MissingLists, rule.Name)
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("规则组 %s 缺少 %s，已跳过", rule.Name, filepath.Base(listPath)))
				continue
			}
			return CompileResult{}, err
		}
		content, err := marshalRuleSet(parsed)
		if err != nil {
			return CompileResult{}, fmt.Errorf("规则组 %s: %w", rule.Name, err)
		}
		outputPath := filepath.Join(ruleSetDir, rule.Name+".json")
		if err := provider.WriteAtomic(outputPath, content, 0o600); err != nil {
			return CompileResult{}, err
		}
		ruleSets = append(ruleSets, fragmentRuleSet{
			Type: "local", Tag: rule.Name, Format: "source", Path: outputPath,
		})
		rules = append(rules, fragmentRule{
			RuleSet: rule.Name, Action: "route", Outbound: rule.Group,
		})
		result.RuleSetCount++
		result.EntryCount += parsed.Count
	}

	result.GroupCount = len(config.Groups)

	// 3. 写出片段
	document := fragment{
		Route:     fragmentRoute{RuleSet: ruleSets, Rules: rules},
		Outbounds: outbounds,
		Providers: providers,
	}
	if err := writeFragment(result.FragmentPath, document); err != nil {
		return CompileResult{}, err
	}
	return result, nil
}

// resolveProviderTags 把 Catalog 分组 ID 解析成运行时 Provider tag。
func resolveProviderTags(ctx context.Context, groupIDs []string, options CompileOptions) ([]string, []string) {
	tags := make([]string, 0, len(groupIDs))
	var missing []string
	for _, id := range groupIDs {
		if options.ProviderTag == nil {
			tags = append(tags, id)
			continue
		}
		tag, err := options.ProviderTag(ctx, id)
		if err != nil {
			missing = append(missing, id)
			continue
		}
		tags = append(tags, tag)
	}
	return tags, missing
}

func marshalRuleSet(parsed ListParseResult) ([]byte, error) {
	content, err := json.Marshal(parsed.ToRuleSet(), jsontext.WithIndent("  "))
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func writeFragment(path string, document fragment) error {
	content, err := json.Marshal(document, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return provider.WriteAtomic(path, content, 0o600)
}

// writeAtomic 原子写入文本文件。
func writeAtomic(path string, content []byte) error {
	return provider.WriteAtomic(path, content, 0o600)
}
