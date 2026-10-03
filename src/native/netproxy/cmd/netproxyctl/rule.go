package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/catalog"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/policy"
)

// rule 命令管理规则组：一个规则组对应 rules/local/<名称>.list 与一条路由规则。
//
// 用法:
//
//	netproxyctl rule list
//	netproxyctl rule show <名称>
//	netproxyctl rule set <名称> --group <节点组> [--file <文件>]
//	netproxyctl rule remove <名称>
//	netproxyctl rule check <名称>
//	netproxyctl rule fields
func (c *cli) rule(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	action := args[0]
	flags := newFlagSet("rule " + action)
	group := flags.String("group", "", "规则组使用的节点组")
	file := flags.String("file", "", "从该文件导入规则内容，留空表示仅登记")
	enabled := flags.String("enabled", "", "是否启用：true/false")
	positionals, err := parseFlagsAnywhere(flags, args[1:])
	if err != nil {
		return err
	}
	options := c.options

	switch action {
	case "list":
		return c.ruleList(options)
	case "fields":
		writeJSON(os.Stdout, result{
			Schema: 1, OK: true, Code: "rule.fields", Message: "受支持的 .list 字段",
			Data: map[string]any{
				"fields": policy.SupportedListFields(),
				"sample": []string{
					"# 注释以 # 开头",
					"domain_suffix:openai.com",
					"domain_keyword:openai",
					"ip_cidr:23.102.140.0/22",
					"process_name:com.openai.chatgpt",
				},
				"directory": policy.ListDir(options.SingBoxDir),
			},
		})
		return nil
	case "show":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl rule show <名称>")
		}
		return c.ruleShow(options, positionals[0])
	case "check":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl rule check <名称>")
		}
		return c.ruleCheck(options, positionals[0])
	case "set":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl rule set <名称> --group <节点组> [--file <文件>]")
		}
		return c.ruleSet(ctx, options, positionals[0], *group, *file, *enabled)
	case "remove":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl rule remove <名称>")
		}
		return c.ruleRemove(ctx, options, positionals[0])
	default:
		return usageError("用法: netproxyctl rule list|show|set|remove|check|fields")
	}
}

func (c *cli) ruleList(options moduleapp.Options) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	type entry struct {
		Name    string `json:"name"`
		Group   string `json:"group"`
		Enabled bool   `json:"enabled"`
		List    string `json:"list"`
		Exists  bool   `json:"exists"`
		Count   int    `json:"count"`
		Error   string `json:"error,omitempty"`
	}
	entries := make([]entry, 0, len(config.Rules))
	for _, rule := range config.Rules {
		item := entry{
			Name:    rule.Name,
			Group:   rule.Group,
			Enabled: rule.IsEnabled(),
			List:    policy.ListPath(options.SingBoxDir, rule.Name),
		}
		parsed, err := policy.ParseListFile(item.List)
		switch {
		case err == nil:
			item.Exists = true
			item.Count = parsed.Count
		case errors.Is(err, os.ErrNotExist):
		default:
			item.Error = err.Error()
		}
		entries = append(entries, item)
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "rule.list", Message: "规则组列表",
		Data: map[string]any{
			"rules":      entries,
			"list_dir":   policy.ListDir(options.SingBoxDir),
			"config":     policy.ConfigPath(options.SingBoxDir),
			"rule_count": len(entries),
		},
	})
	return nil
}

func (c *cli) ruleShow(options moduleapp.Options, name string) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	rule, found := config.FindRule(name)
	if !found {
		return &resultError{Code: "rule.not_found", Message: "规则组不存在: " + name}
	}
	path := policy.ListPath(options.SingBoxDir, name)
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &resultError{Code: "rule.list_missing", Message: fmt.Sprintf("规则组 %s 缺少 %s", name, path)}
		}
		return err
	}
	parsed, err := policy.ParseListFile(path)
	if err != nil {
		return err
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "rule.show", Message: "规则组内容",
		Data: map[string]any{
			"name":    rule.Name,
			"group":   rule.Group,
			"enabled": rule.IsEnabled(),
			"path":    path,
			"count":   parsed.Count,
			"content": string(content),
			"rules":   parsed.ToRuleSet(),
		},
	})
	return nil
}

func (c *cli) ruleCheck(options moduleapp.Options, name string) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	rule, found := config.FindRule(name)
	if !found {
		return &resultError{Code: "rule.not_found", Message: "规则组不存在: " + name}
	}
	path := policy.ListPath(options.SingBoxDir, name)
	parsed, err := policy.ParseListFile(path)
	if err != nil {
		return &resultError{Code: "rule.invalid", Message: err.Error()}
	}
	group, groupFound := config.FindGroup(rule.Group)
	if !groupFound {
		return &resultError{
			Code:    "rule.group_missing",
			Message: fmt.Sprintf("规则组 %s 引用的节点组 %s 不存在", name, rule.Group),
		}
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "rule.checked", Message: "规则组检查通过",
		Data: map[string]any{
			"name": name, "group": rule.Group, "count": parsed.Count,
			"path": path, "strategy": group.EffectiveStrategy(),
		},
	})
	return nil
}

func (c *cli) ruleSet(ctx context.Context, options moduleapp.Options, name, group, sourceFile, enabled string) error {
	if strings.TrimSpace(name) == "" {
		return usageError("规则组名不能为空")
	}
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	rule, exists := config.FindRule(name)
	if !exists {
		rule = policy.RuleSet{Name: name}
	}
	if group != "" {
		if _, found := config.FindGroup(group); !found {
			return &resultError{
				Code:    "rule.group_missing",
				Message: fmt.Sprintf("节点组不存在: %s（可先用 group list 查看）", group),
			}
		}
		rule.Group = group
	}
	if rule.Group == "" {
		return &resultError{Code: "usage.invalid", Message: "必须通过 --group 指定节点组", Status: 2}
	}
	switch strings.ToLower(strings.TrimSpace(enabled)) {
	case "":
	case "true", "1", "yes":
		value := true
		rule.Enabled = &value
	case "false", "0", "no":
		value := false
		rule.Enabled = &value
	default:
		return usageError("--enabled 只接受 true/false")
	}

	// 写入 .list 内容
	listPath := policy.ListPath(options.SingBoxDir, name)
	if sourceFile != "" {
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			return err
		}
		if _, err := policy.ParseList(strings.NewReader(string(content))); err != nil {
			return &resultError{Code: "rule.invalid", Message: fmt.Sprintf("规则内容无效: %v", err)}
		}
		if err := os.MkdirAll(filepath.Dir(listPath), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(listPath, content, 0o600); err != nil {
			return err
		}
	} else if _, err := os.Stat(listPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(listPath), 0o700); err != nil {
			return err
		}
		header := "# " + name + " 规则组\n# 一行一条，字段与 sing-box rule-set 一致\n# 例如: domain_suffix:example.com\n"
		if err := os.WriteFile(listPath, []byte(header), 0o600); err != nil {
			return err
		}
	}

	created := config.UpsertRule(rule)
	if err := policy.Save(policy.ConfigPath(options.SingBoxDir), config); err != nil {
		return err
	}
	if err := c.recompilePolicy(ctx, options); err != nil {
		return err
	}
	message := "规则组已更新"
	if created {
		message = "规则组已创建"
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "rule.set", Message: message,
		Data: map[string]any{
			"name": name, "group": rule.Group, "enabled": rule.IsEnabled(),
			"path": listPath, "created": created,
		},
	})
	return nil
}

func (c *cli) ruleRemove(ctx context.Context, options moduleapp.Options, name string) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	if err := config.RemoveRule(name); err != nil {
		return &resultError{Code: "rule.not_found", Message: err.Error()}
	}
	if err := policy.Save(policy.ConfigPath(options.SingBoxDir), config); err != nil {
		return err
	}
	// 生成的规则集与 .list 一并清理，避免残留。
	_ = os.Remove(filepath.Join(policy.RuleSetDir(options.SingBoxDir), name+".json"))
	listPath := policy.ListPath(options.SingBoxDir, name)
	_ = os.Remove(listPath)
	if err := c.recompilePolicy(ctx, options); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "rule.removed", Message: "规则组已删除",
		Data: map[string]any{"name": name, "list": listPath},
	})
	return nil
}

// recompilePolicy 重新编译运行时片段，供规则组与节点组命令共用。
func (c *cli) recompilePolicy(ctx context.Context, options moduleapp.Options) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	_, err = policy.Compile(ctx, config, policy.CompileOptions{
		SingBoxDir:  options.SingBoxDir,
		RuntimeDir:  options.RuntimeDir,
		CatalogRoot: options.CatalogRoot,
		ProviderTag: func(ctx context.Context, groupID string) (string, error) {
			return catalog.RuntimeTag(ctx, options.CatalogRoot, groupID)
		},
		ProviderPath: func(ctx context.Context, providerTag string) (string, error) {
			return catalog.ProviderPathByRuntimeTag(ctx, options.CatalogRoot, providerTag)
		},
	})
	return err
}

// sortedKeys 返回 map 的排序键，用于稳定输出。
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
