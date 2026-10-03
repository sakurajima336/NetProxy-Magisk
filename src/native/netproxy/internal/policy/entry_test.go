package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEntryAcceptsCommonInput(t *testing.T) {
	cases := []struct {
		kind  EntryKind
		input string
		want  string
	}{
		{EntrySuffix, "Example.COM", "example.com"},
		{EntrySuffix, "https://example.com/path?x=1", "example.com"},
		{EntrySuffix, "example.com.", "example.com"},
		{EntryDomain, "api.openai.com", "api.openai.com"},
		{EntryKeyword, "OpenAI", "openai"},
		{EntryIP, "1.2.3.4", "1.2.3.4"},
		{EntryIP, "1.2.3.0/24", "1.2.3.0/24"},
		{EntryIP, "2001:db8::/32", "2001:db8::/32"},
	}
	for _, testCase := range cases {
		got, err := ValidateEntry(testCase.kind, testCase.input)
		if err != nil {
			t.Fatalf("%s %q 应被接受: %v", testCase.kind, testCase.input, err)
		}
		if got != testCase.want {
			t.Fatalf("%s %q 应规范化为 %q，实际 %q", testCase.kind, testCase.input, testCase.want, got)
		}
	}
}

func TestValidateEntryRejectsBadInput(t *testing.T) {
	cases := []struct {
		kind  EntryKind
		input string
	}{
		{EntrySuffix, "notadomain"},
		{EntrySuffix, "has space.com"},
		{EntrySuffix, ""},
		{EntryDomain, "just-a-word"},
		{EntryIP, "999.1.1.1"},
		{EntryIP, "example.com"},
	}
	for _, testCase := range cases {
		if _, err := ValidateEntry(testCase.kind, testCase.input); err == nil {
			t.Fatalf("%s %q 应被拒绝", testCase.kind, testCase.input)
		}
	}
}

func TestAddAndListEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules", "OpenAI.list")
	added, list, err := AddEntries(path, []Entry{
		{Kind: EntrySuffix, Value: "openai.com"},
		{Kind: EntrySuffix, Value: "chatgpt.com"},
		{Kind: EntryIP, Value: "23.102.140.0/22"},
	})
	if err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if added != 3 || len(list.Entries) != 3 {
		t.Fatalf("应添加 3 条，实际 added=%d count=%d", added, len(list.Entries))
	}

	// 重复添加应被跳过
	added, list, err = AddEntries(path, []Entry{{Kind: EntrySuffix, Value: "openai.com"}})
	if err != nil {
		t.Fatalf("重复添加失败: %v", err)
	}
	if added != 0 || len(list.Entries) != 3 {
		t.Fatalf("重复项应被跳过，实际 added=%d count=%d", added, len(list.Entries))
	}
}

func TestRemoveEntriesSingleAndMultiple(t *testing.T) {
	path := filepath.Join(t.TempDir(), "OpenAI.list")
	if _, _, err := AddEntries(path, []Entry{
		{Kind: EntrySuffix, Value: "a.com"},
		{Kind: EntrySuffix, Value: "b.com"},
		{Kind: EntrySuffix, Value: "c.com"},
	}); err != nil {
		t.Fatal(err)
	}

	// 单个删除
	removed, list, err := RemoveEntries(path, []Entry{{Kind: EntrySuffix, Value: "b.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(list.Entries) != 2 {
		t.Fatalf("单个删除失败: removed=%d count=%d", removed, len(list.Entries))
	}

	// 多选删除
	removed, list, err = RemoveEntries(path, []Entry{
		{Kind: EntrySuffix, Value: "a.com"},
		{Kind: EntrySuffix, Value: "c.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 || len(list.Entries) != 0 {
		t.Fatalf("多选删除失败: removed=%d count=%d", removed, len(list.Entries))
	}
}

func TestEntriesPreserveHandwrittenFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Mixed.list")
	// 用户手写了高级字段
	original := "# 手写\nport:443\nprocess_name:com.example.app\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	// 客户端只加域名
	if _, _, err := AddEntries(path, []Entry{{Kind: EntrySuffix, Value: "example.com"}}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "domain_suffix:example.com") {
		t.Fatalf("应写入域名条目:\n%s", text)
	}
	// 手写的高级字段必须保留，不能被客户端覆盖
	if !strings.Contains(text, "port:443") || !strings.Contains(text, "process_name:com.example.app") {
		t.Fatalf("手写字段被覆盖了:\n%s", text)
	}

	// 解析时其他字段应计入 Other
	list, err := ParseEntriesFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if list.Other != 2 {
		t.Fatalf("手写字段应计入 Other=2，实际 %d", list.Other)
	}
	if len(list.Entries) != 1 {
		t.Fatalf("应有 1 条域名条目，实际 %d", len(list.Entries))
	}
}

func TestParseEntriesKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Kinds.list")
	content := "domain:exact.com\ndomain_suffix:suf.com\ndomain_keyword:kw\nip_cidr:1.2.3.0/24\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := ParseEntriesFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 4 {
		t.Fatalf("应有 4 条，实际 %d", len(list.Entries))
	}
	kinds := map[EntryKind]bool{}
	for _, entry := range list.Entries {
		kinds[entry.Kind] = true
	}
	for _, want := range []EntryKind{EntryDomain, EntrySuffix, EntryKeyword, EntryIP} {
		if !kinds[want] {
			t.Fatalf("缺少类型 %s: %+v", want, list.Entries)
		}
	}
}

func TestAddEntriesToMissingFileCreatesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "New.list")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("测试前提：文件不应存在")
	}
	added, list, err := AddEntries(path, []Entry{{Kind: EntrySuffix, Value: "new.com"}})
	if err != nil {
		t.Fatalf("向不存在的文件添加应成功: %v", err)
	}
	if added != 1 || len(list.Entries) != 1 {
		t.Fatalf("应创建并写入 1 条: added=%d count=%d", added, len(list.Entries))
	}
}