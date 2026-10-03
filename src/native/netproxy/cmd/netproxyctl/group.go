package main

import (
	"context"
	"os"
	"strings"

	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/policy"
)

// group 命令管理节点组：一个节点组描述"用哪些订阅、哪些单独节点"。
//
// 用法:
//
//	netproxyctl group list
//	netproxyctl group show <名称>
//	netproxyctl group set <名称> [--providers a,b] [--nodes c/d,e/f] [--include 正则] [--exclude 正则] [--strategy urltest|manual] [--default <节点>]
//	netproxyctl group remove <名称>
func (c *cli) group(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	action := args[0]
	flags := newFlagSet("group " + action)
	providers := flags.String("providers", "", "整份订阅的 Catalog 分组 ID，逗号分隔")
	nodes := flags.String("nodes", "", "单独节点，<分组 ID>/<节点标签>，逗号分隔")
	include := flags.String("include", "", "正则筛选，匹配节点名")
	exclude := flags.String("exclude", "", "正则排除，优先级高于 include")
	strategy := flags.String("strategy", "", "测速策略：urltest（默认）或 manual")
	defaultNode := flags.String("default", "", "manual 策略下的默认选中节点")
	positionals, err := parseFlagsAnywhere(flags, args[1:])
	if err != nil {
		return err
	}
	options := c.options

	switch action {
	case "list":
		return c.groupList(options)
	case "show":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl group show <名称>")
		}
		return c.groupShow(options, positionals[0])
	case "set":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl group set <名称> [--providers ...] [--nodes ...]")
		}
		return c.groupSet(ctx, options, positionals[0], groupUpdate{
			providers: *providers, nodes: *nodes, include: *include,
			exclude: *exclude, strategy: *strategy, defaultNode: *defaultNode,
		})
	case "remove":
		if len(positionals) == 0 {
			return usageError("用法: netproxyctl group remove <名称>")
		}
		return c.groupRemove(ctx, options, positionals[0])
	default:
		return usageError("用法: netproxyctl group list|show|set|remove")
	}
}

// groupUpdate 收集一次节点组更新中显式传入的字段。
// 未传入的字段保持原值，避免误清空。
type groupUpdate struct {
	providers   string
	nodes       string
	include     string
	exclude     string
	strategy    string
	defaultNode string
}

func (c *cli) groupList(options moduleapp.Options) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	type entry struct {
		Tag       string   `json:"tag"`
		Providers []string `json:"providers,omitempty"`
		Nodes     []string `json:"nodes,omitempty"`
		Include   string   `json:"include,omitempty"`
		Exclude   string   `json:"exclude,omitempty"`
		Strategy  string   `json:"strategy"`
		Default   string   `json:"default,omitempty"`
		UsedBy    []string `json:"used_by,omitempty"`
	}
	entries := make([]entry, 0, len(config.Groups))
	for _, group := range config.Groups {
		item := entry{
			Tag: group.Tag, Providers: group.Providers, Nodes: group.Nodes,
			Include: group.Include, Exclude: group.Exclude,
			Strategy: group.EffectiveStrategy(), Default: group.Default,
		}
		for _, rule := range config.Rules {
			if rule.Group == group.Tag {
				item.UsedBy = append(item.UsedBy, rule.Name)
			}
		}
		entries = append(entries, item)
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "group.list", Message: "节点组列表",
		Data: map[string]any{
			"groups":      entries,
			"config":      policy.ConfigPath(options.SingBoxDir),
			"group_count": len(entries),
		},
	})
	return nil
}

func (c *cli) groupShow(options moduleapp.Options, tag string) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	group, found := config.FindGroup(tag)
	if !found {
		return &resultError{Code: "group.not_found", Message: "节点组不存在: " + tag}
	}
	var usedBy []string
	for _, rule := range config.Rules {
		if rule.Group == tag {
			usedBy = append(usedBy, rule.Name)
		}
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "group.show", Message: "节点组详情",
		Data: map[string]any{
			"tag": group.Tag, "providers": group.Providers, "nodes": group.Nodes,
			"include": group.Include, "exclude": group.Exclude,
			"strategy": group.EffectiveStrategy(), "default": group.Default,
			"used_by": usedBy,
		},
	})
	return nil
}

func (c *cli) groupSet(ctx context.Context, options moduleapp.Options, tag string, update groupUpdate) error {
	if strings.TrimSpace(tag) == "" {
		return usageError("节点组名不能为空")
	}
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	group, exists := config.FindGroup(tag)
	if !exists {
		group = policy.Group{Tag: tag}
	}
	// 只有显式传入的字段才覆盖，避免 --include 之外的调用把已有配置清空。
	if update.providers != "" {
		group.Providers = splitCSV(update.providers)
	}
	if update.nodes != "" {
		group.Nodes = splitCSV(update.nodes)
	}
	if update.include != "" {
		group.Include = update.include
	}
	if update.exclude != "" {
		group.Exclude = update.exclude
	}
	if update.strategy != "" {
		group.Strategy = strings.ToLower(strings.TrimSpace(update.strategy))
	}
	if update.defaultNode != "" {
		group.Default = update.defaultNode
	}

	created := config.UpsertGroup(group)
	if err := policy.Save(policy.ConfigPath(options.SingBoxDir), config); err != nil {
		return err
	}
	if err := c.recompilePolicy(ctx, options); err != nil {
		return err
	}
	message := "节点组已更新"
	if created {
		message = "节点组已创建"
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "group.set", Message: message,
		Data: map[string]any{
			"tag": tag, "providers": group.Providers, "nodes": group.Nodes,
			"strategy": group.EffectiveStrategy(), "created": created,
		},
	})
	return nil
}

func (c *cli) groupRemove(ctx context.Context, options moduleapp.Options, tag string) error {
	config, err := policy.Load(policy.ConfigPath(options.SingBoxDir))
	if err != nil {
		return err
	}
	if err := config.RemoveGroup(tag); err != nil {
		return &resultError{Code: "group.remove_failed", Message: err.Error()}
	}
	if err := policy.Save(policy.ConfigPath(options.SingBoxDir), config); err != nil {
		return err
	}
	if err := c.recompilePolicy(ctx, options); err != nil {
		return err
	}
	writeJSON(os.Stdout, result{
		Schema: 1, OK: true, Code: "group.removed", Message: "节点组已删除",
		Data: map[string]any{"tag": tag},
	})
	return nil
}

// splitCSV 拆分逗号分隔列表并去除空白项。
func splitCSV(raw string) []string {
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
