package policy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFragmentAcceptedBySingBox 让真实 sing-box 校验生成的运行时片段。
//
// 需要设置 NETPROXY_SINGBOX_BIN 指向 sing-box 二进制；未设置时跳过，
// 这样常规单元测试不依赖外部二进制。
func TestFragmentAcceptedBySingBox(t *testing.T) {
	binary := os.Getenv("NETPROXY_SINGBOX_BIN")
	if binary == "" {
		t.Skip("未设置 NETPROXY_SINGBOX_BIN，跳过 sing-box 集成校验")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("sing-box 二进制不可用: %v", err)
	}

	root := t.TempDir()
	singBoxDir := filepath.Join(root, "singbox")
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(ListDir(singBoxDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// 1. 节点文件：供 Provider 引用
	providerPath := filepath.Join(root, "nodes.json")
	nodeContent := `{"outbounds":[{"type":"socks","tag":"赔钱日本06","server":"127.0.0.1","server_port":1080,"version":"5"}]}`
	if err := os.WriteFile(providerPath, []byte(nodeContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// 2. providers.json：模拟 Catalog 生成的运行时 Provider
	providersPath := filepath.Join(runtimeDir, "providers.json")
	providersContent := `{"providers":[{"type":"local","tag":"赔钱","path":"` + providerPath + `"}]}`
	if err := os.WriteFile(providersPath, []byte(providersContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. 策略配置：节点组同时使用整订阅与单独节点，规则组分别指向它
	config := Config{
		Version: 1,
		Groups: []Group{
			{
				Tag:       "AI节点组",
				Providers: []string{"providers.json"},
				Nodes:     []string{"赔钱/赔钱日本06"},
				Exclude:   `(?i)(港|HK)`,
			},
			{Tag: "Claude节点组", Strategy: "manual", Nodes: []string{"赔钱/赔钱日本06"}},
		},
		Rules: []RuleSet{
			{Name: "OpenAI", Group: "AI节点组"},
			{Name: "Claude", Group: "Claude节点组"},
		},
	}
	lists := map[string]string{
		"OpenAI": "domain_suffix:openai.com\ndomain_suffix:chatgpt.com\nip_cidr:23.102.140.0/22\n",
		"Claude": "domain_suffix:anthropic.com\ndomain_suffix:claude.ai\nprocess_name:com.anthropic.claude\n",
	}
	for name, content := range lists {
		if err := os.WriteFile(ListPath(singBoxDir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Save(ConfigPath(singBoxDir), config); err != nil {
		t.Fatal(err)
	}

	result, err := Compile(context.Background(), config, CompileOptions{
		SingBoxDir: singBoxDir,
		RuntimeDir: runtimeDir,
		ProviderTag: func(_ context.Context, _ string) (string, error) {
			return "赔钱", nil
		},
		ProviderPath: func(_ context.Context, _ string) (string, error) {
			return providerPath, nil
		},
	})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}

	// 4. 主配置：模拟 NetProxy 的静态配置
	mainPath := filepath.Join(singBoxDir, "config.json")
	mainContent := `{
  "log": {"level": "info", "timestamp": true},
  "dns": {"servers": [{"tag": "local", "type": "udp", "server": "223.5.5.5"}], "final": "local"},
  "inbounds": [{"type": "mixed", "tag": "in", "listen": "127.0.0.1", "listen_port": 18099}],
  "outbounds": [{"type": "direct", "tag": "direct"}, {"type": "direct", "tag": "Proxy"}],
  "route": {"default_domain_resolver": "local", "final": "Proxy"}
}`
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// 5. 交给真实 sing-box 校验
	command := exec.Command(binary, "check",
		"-c", mainPath,
		"-c", providersPath,
		"-c", result.FragmentPath,
	)
	command.Dir = singBoxDir
	output, err := command.CombinedOutput()
	if err != nil {
		fragment, _ := os.ReadFile(result.FragmentPath)
		t.Fatalf("sing-box 校验失败: %v\n输出: %s\n片段:\n%s", err, output, fragment)
	}
	t.Logf("sing-box 校验通过: %s", output)
}
