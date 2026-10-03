package policy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCompileFixture 建立一次编译所需的临时目录结构。
// 返回 singBoxDir、runtimeDir 和源订阅的节点文件路径。
func setupCompileFixture(t *testing.T, config Config, lists map[string]string) (string, string) {
	t.Helper()
	root := t.TempDir()
	singBoxDir := filepath.Join(root, "singbox")
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(ListDir(singBoxDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range lists {
		path := ListPath(singBoxDir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(ConfigPath(singBoxDir), config); err != nil {
		t.Fatal(err)
	}
	return singBoxDir, runtimeDir
}

// sourceProviderPath 写入一个模拟订阅的节点文件并返回其路径。
func sourceProviderPath(t *testing.T, singBoxDir string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(singBoxDir), "source-provider.json")
	content := `{"outbounds":[
		{"type":"socks","tag":"赔钱日本06","server":"127.0.0.1","server_port":1080,"version":"5"},
		{"type":"socks","tag":"赔钱香港01","server":"127.0.0.1","server_port":1081,"version":"5"}
	]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// testCompileOptions 返回带 ProviderPath 的编译选项。
func testCompileOptions(t *testing.T, singBoxDir, runtimeDir string) CompileOptions {
	t.Helper()
	sourcePath := sourceProviderPath(t, singBoxDir)
	return CompileOptions{
		SingBoxDir: singBoxDir,
		RuntimeDir: runtimeDir,
		ProviderTag: func(_ context.Context, _ string) (string, error) {
			return "赔钱", nil
		},
		ProviderPath: func(_ context.Context, _ string) (string, error) {
			return sourcePath, nil
		},
	}
}

func TestCompileGeneratesFragment(t *testing.T) {
	config := Config{
		Version: 1,
		Groups: []Group{
			{Tag: "AI节点组", Providers: []string{"grp-ai"}, Nodes: []string{"grp-ai/赔钱日本06"}},
			{Tag: "手动组", Strategy: "manual", Nodes: []string{"grp-ai/赔钱香港01"}, Default: "grp-ai/赔钱香港01"},
		},
		Rules: []RuleSet{
			{Name: "OpenAI", Group: "AI节点组"},
			{Name: "Claude", Group: "AI节点组"},
		},
	}
	lists := map[string]string{
		"OpenAI": "domain_suffix:openai.com\ndomain_suffix:chatgpt.com\n",
		"Claude": "domain_suffix:anthropic.com\n",
	}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, lists)

	result, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir))
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if result.GroupCount != 2 {
		t.Fatalf("节点组数应为 2，实际 %d", result.GroupCount)
	}
	if result.RuleSetCount != 2 {
		t.Fatalf("规则组数应为 2，实际 %d", result.RuleSetCount)
	}
	if result.EntryCount != 3 {
		t.Fatalf("条目数应为 3，实际 %d", result.EntryCount)
	}

	// 片段文件应存在
	if _, err := os.Stat(result.FragmentPath); err != nil {
		t.Fatalf("片段文件未生成: %v", err)
	}
	// 生成的规则集应存在
	for _, name := range []string{"OpenAI", "Claude"} {
		path := filepath.Join(RuleSetDir(singBoxDir), name+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("规则集未生成 %s: %v", name, err)
		}
	}

	content, err := os.ReadFile(result.FragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	// 订阅应解析成运行时 tag
	if !strings.Contains(text, `"赔钱"`) {
		t.Fatalf("订阅应解析为运行时 tag:\n%s", text)
	}
	// 单独节点应抽取成独立 Provider，并出现在 selector 的 providers 中
	if !strings.Contains(text, PickedProviderTag("AI节点组")) {
		t.Fatalf("单独节点应生成 Picked Provider:\n%s", text)
	}
	// 单独节点不能直接写在 outbounds 里（否则会 dependency not found）
	if strings.Contains(text, `"outbounds": [
        "赔钱/`) {
		t.Fatalf("单独节点不应直接放进 outbounds:\n%s", text)
	}
	// 规则集路径应为绝对路径，避免工作目录歧义
	if !strings.Contains(text, filepath.Join(singBoxDir, "rules", "policy", "OpenAI.json")) {
		t.Fatalf("规则集路径应为绝对路径:\n%s", text)
	}
}

func TestCompileExtractsPickedNodes(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI", Providers: []string{"grp-ai"}, Nodes: []string{"grp-ai/赔钱日本06"}}},
	}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, nil)
	result, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir))
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	// 抽取出的 Provider 文件应只含被选中的节点
	pickedPath := filepath.Join(RuleSetDir(singBoxDir), "picked-AI.json")
	content, err := os.ReadFile(pickedPath)
	if err != nil {
		t.Fatalf("抽取文件未生成: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "赔钱日本06") {
		t.Fatalf("应包含选中的节点:\n%s", text)
	}
	if strings.Contains(text, "赔钱香港01") {
		t.Fatalf("不应包含未选中的节点:\n%s", text)
	}
	if result.FragmentPath == "" {
		t.Fatal("片段路径不应为空")
	}
}

func TestCompileUsesUrltestAndSelector(t *testing.T) {
	config := Config{
		Version: 1,
		Groups: []Group{
			{Tag: "自动组"},
			{Tag: "手动组", Strategy: "manual", Default: "direct"},
		},
	}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, nil)
	result, err := Compile(context.Background(), config, CompileOptions{
		SingBoxDir: singBoxDir,
		RuntimeDir: runtimeDir,
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	content, _ := os.ReadFile(result.FragmentPath)
	text := string(content)
	if !strings.Contains(text, `"type": "urltest"`) {
		t.Fatalf("默认策略应生成 urltest:\n%s", text)
	}
	if !strings.Contains(text, `"type": "selector"`) {
		t.Fatalf("manual 策略应生成 selector:\n%s", text)
	}
	// 空组应回退到 Proxy，避免空组导致启动失败
	if !strings.Contains(text, `"Proxy"`) {
		t.Fatalf("空节点组应回退到 Proxy:\n%s", text)
	}
}

func TestCompileSkipsMissingList(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules:   []RuleSet{{Name: "OpenAI", Group: "AI"}, {Name: "没有这个文件", Group: "AI"}},
	}
	lists := map[string]string{"OpenAI": "domain_suffix:openai.com\n"}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, lists)

	result, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir))
	if err != nil {
		t.Fatalf("缺少 .list 不应导致编译失败: %v", err)
	}
	if result.RuleSetCount != 1 {
		t.Fatalf("只应生成 1 个规则集，实际 %d", result.RuleSetCount)
	}
	if len(result.MissingLists) != 1 || result.MissingLists[0] != "没有这个文件" {
		t.Fatalf("应报告缺失的规则组: %+v", result.MissingLists)
	}
}

func TestCompileSkipsDisabledRule(t *testing.T) {
	disabled := false
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules: []RuleSet{
			{Name: "启用", Group: "AI"},
			{Name: "停用", Group: "AI", Enabled: &disabled},
		},
	}
	lists := map[string]string{
		"启用": "domain_suffix:a.com\n",
		"停用": "domain_suffix:b.com\n",
	}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, lists)

	result, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir))
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if result.RuleSetCount != 1 {
		t.Fatalf("停用的规则组不应生成，实际 %d", result.RuleSetCount)
	}
}

func TestCompileReportsMissingProvider(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI", Providers: []string{"missing-grp"}}},
	}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, nil)

	result, err := Compile(context.Background(), config, CompileOptions{
		SingBoxDir: singBoxDir, RuntimeDir: runtimeDir,
		ProviderTag: func(_ context.Context, id string) (string, error) {
			return "", os.ErrNotExist
		},
		ProviderPath: func(_ context.Context, _ string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if len(result.MissingProviders) != 1 {
		t.Fatalf("应报告缺失的订阅: %+v", result.MissingProviders)
	}
}

func TestCompileRejectsInvalidConfig(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules:   []RuleSet{{Name: "OpenAI", Group: "不存在的组"}},
	}
	// 绕过 Save 的校验，直接落盘一份非法配置，验证 Compile 自身会拒绝。
	root := t.TempDir()
	singBoxDir := filepath.Join(root, "singbox")
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(ListDir(singBoxDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir)); err == nil {
		t.Fatal("非法配置应拒绝编译")
	}
}

func TestCompileRejectsBadList(t *testing.T) {
	config := Config{
		Version: 1,
		Groups:  []Group{{Tag: "AI"}},
		Rules:   []RuleSet{{Name: "OpenAI", Group: "AI"}},
	}
	lists := map[string]string{"OpenAI": "DOMAIN-SUFFIX,openai.com\n"}
	singBoxDir, runtimeDir := setupCompileFixture(t, config, lists)

	if _, err := Compile(context.Background(), config, testCompileOptions(t, singBoxDir, runtimeDir)); err == nil {
		t.Fatal("非法 .list 应导致编译失败")
	}
}
