# NetProxy Fork 变更说明

本文件记录本 fork 相对上游 [Fanju6/NetProxy-Magisk](https://github.com/Fanju6/NetProxy-Magisk) 新增的功能与约束。

**改动代码前请先读 [AGENTS.md](AGENTS.md)**，本文件只补充 fork 特有的部分。

---

## 这个 fork 加了什么

一个功能：**规则组 / 节点组管理**。

解决的问题：上游只能靠手写 `config/singbox/config.json` 的 `route.rules` 与 `outbounds` 来分流，加一个域名要改 JSON、还要自己拼 selector。本 fork 把这件事变成「点界面」：

- **节点组**：描述"一组节点" = 若干**整份订阅** + 若干**单独挑选的节点** + 可选正则筛选
- **规则组**：一组规则（`.list` 文件）→ 指定走哪个**节点组**

典型用法：新建「AI节点组」= 勾选一份贵的订阅 + 单独勾几个便宜的日本节点，再建「OpenAI」规则组指向它。以后加域名只改 `.list`，加节点只勾选，都不碰主配置。

---

## 目录与文件

### 新增

```text
src/native/netproxy/internal/policy/      规则组/节点组核心
├── config.go     数据模型、校验、groups.json 读写
├── list.go       .list 解析器（sing-box 字段语法）
├── compile.go    编译成 sing-box 运行时片段
└── *_test.go     单元测试 + 真实 sing-box 集成测试

src/native/netproxy/cmd/netproxyctl/
├── group.go      group 命令
├── rule.go       rule 命令
└── flags.go      parseFlagsAnywhere：允许 flag 出现在位置参数之后

src/android/app/src/main/java/com/fanjv/netproxy/feature/policy/
├── model/PolicyModels.kt              数据模型 + 名称校验
├── data/PolicyRepository.kt           调用 netproxyctl group/rule
└── presentation/
    ├── PolicyViewModel.kt             状态与读写
    ├── PolicyScreen.kt                分组页（节点组 / 规则组两个分区）
    ├── PolicyGroupEditScreen.kt       节点组编辑（勾订阅 + 勾节点 + 正则）
    └── PolicyRuleEditScreen.kt        规则组编辑 + 内容查看
```

### 用户侧文件（设备上）

```text
/data/adb/modules/netproxy/config/singbox/
├── groups.json                  节点组与规则组的登记表（GUI/CLI 维护）
└── rules/local/<规则组名>.list  规则内容（用户维护）

/data/adb/modules/netproxy/config/singbox/rules/policy/   自动生成，勿手改
├── <规则组名>.json              由 .list 编译出的 rule-set
└── picked-<节点组名>.json       由"单独节点"抽取出的 Provider

/data/adb/modules/netproxy/runtime/policy.json             自动生成，勿手改
```

---

## 命令

```text
netproxyctl group list|show|set|remove
netproxyctl rule list|show|set|remove|check|fields
```

示例：

```sh
# 节点组：整份订阅 + 单独节点 + 排除香港
su -c '/data/adb/modules/netproxy/netproxyctl group set AI节点组 \
    --providers <分组ID> --nodes "<分组ID>/<节点标签>" --exclude "^(?!.*(港|HK)).*$"'

# 规则组：指向节点组
su -c '/data/adb/modules/netproxy/netproxyctl rule set OpenAI --group AI节点组'

# 查看支持哪些 .list 字段
su -c '/data/adb/modules/netproxy/netproxyctl rule fields'
```

`.list` 语法（**sing-box 字段名，一行一条**）：

```text
# 注释
domain_suffix:openai.com
domain_keyword:openai
ip_cidr:23.102.140.0/22
process_name:com.openai.chatgpt
```

> **不是** Clash 语法。`DOMAIN-SUFFIX,xxx.com` 会被拒绝并报「不支持的字段」。
> sing-box 没有任何纯文本规则格式，`.list` 是本 fork 自定义的语法，
> 字段名与 sing-box rule-set 一致，由 `internal/policy/list.go` 解析。

---

## 必须知道的实现约束

以下四条都是**编译和测试都不报错、但运行时会静默出错**的坑，改动前务必理解。

### 1. 单独节点必须做成 Provider，不能放进 selector 的 outbounds

sing-box 在 `StartStatePostStart` 阶段的启动顺序是 **outbound 先、provider 后**（`box.go`）：

```go
adapter.Start(..., StartStatePostStart, s.outbound, s.provider, ...)
```

因此 selector 启动时 provider 还没注册节点。若把节点直接写进 `outbounds`：

```json
{"type": "selector", "providers": ["赔钱"], "outbounds": ["赔钱/日本06"]}
```

`sing-box check` **会通过**，但 `run` 时报：

```text
FATAL start service: dependency[赔钱/日本06] not found for outbound[...]
```

**正确做法**：把单独节点抽取成独立 Provider（`Picked/<节点组名>`），selector 只通过 `providers` 引用。

```json
{
  "outbounds": [{"type": "urltest", "tag": "AI节点组",
                 "providers": ["赔钱", "Picked/AI节点组"]}],
  "providers": [{"type": "local", "tag": "Picked/AI节点组",
                 "path": ".../picked-AI节点组.json"}]
}
```

实现在 `compile.go` 的 `writePickedProvider`。**不要**"优化"成直接写 outbounds。

### 2. 主配置 `config.json` 永不被修改

编译产物走独立文件 `runtime/policy.json`，通过额外的 `-c` 叠加：

```go
exec.Command(singBoxPath, "run",
    "-c", config.json, "-c", providers.json, "-c", outbounds.json,
    "-c", ebpf.json, "-c", policy.json)   // ← 本 fork 新增
```

依据是 sing-box 多配置合并时**数组是追加而非覆盖**（`badjson.MergeJSON`，`disableAppend=false`）。
所以规则与出站都是叠加，不会互相覆盖，用户的主配置保持原样。

新增运行时文件时，必须同步更新这些位置的清单（都硬编码了文件名）：
`internal/module/{app,config,config_transaction,lifecycle,logs}.go`。

### 3. Go 标准 flag 包遇到第一个位置参数就停止解析

```sh
group set AI节点组 --providers x     # --providers 会被忽略！
```

`cmd/netproxyctl/flags.go` 的 `parseFlagsAnywhere` 先把参数重排成"flag 在前"，再交给 flag 包。
**新增带 flag 的命令时用它，不要直接用 `flags.Parse`。**

### 4. 节点引用有 Catalog 格式与运行时格式两种

| 场合 | 格式 | 例子 |
|---|---|---|
| 用户配置 `groups.json` | `<Catalog 分组 ID>/<节点标签>` | `a1b2c3/日本东京06` |
| sing-box 运行时 | `<Provider tag>/<节点标签>` | `赔钱/日本东京06` |

两者必须转换（`resolveNodeRefs`），否则 sing-box 报 `dependency not found`。
Provider tag 由 `catalog.RuntimeTag` 解析（可能带去重后缀 `赔钱 [a1b2c3]`）。

---

## Android 侧约定

- **分组页由开关控制**：设置 → 「显示分组页」，默认关闭（`POLICY_TAB_ENABLED_DEFAULT = false`）。
  分组页固定插在节点页左边，实现见 `MainActivity.kt` 的 `destinations` 构建。
  **功能稳定后移除开关即可让它成为默认入口**，届时删掉 `POLICY_TAB_ENABLED_*` 常量与设置项。
- 数据流保持 `Compose -> ViewModel -> Repository -> NetProxyCtlClient -> netproxyctl`，
  不直接读写 `/data/adb`。
- 编辑态以覆盖层（early return）呈现，避免打断列表滚动位置。
- 节点组编辑页需要节点目录，通过 `CatalogNodesViewModel` 复用，不新建节点仓库。

---

## 验证

```sh
# Go（含 policy 包）
cd src/native/netproxy && go test ./... && go vet ./...

# 用真实 sing-box 校验生成的片段（推荐，能抓出依赖顺序问题）
NETPROXY_SINGBOX_BIN=/path/to/sing-box go test ./internal/policy/... -run TestFragmentAcceptedBySingBox -v

# Android
cd src/android && ./gradlew :app:compileDebugKotlin lintDebug
```

> `TestFragmentAcceptedBySingBox` 只在设置了 `NETPROXY_SINGBOX_BIN` 时执行，否则跳过。
> 它调用 `sing-box check` 校验编译产物，是发现"配置能过 check 但跑不起来"的第一道防线。
> **但注意 `check` 不验证依赖存在性**，第 1 条那个 bug 就是 check 通过、run 才失败，
> 所以改编译逻辑后务必真机或本地 `sing-box run` 实测一次。

### 本地构建（非 CI）

```sh
# netproxyctl（Android arm64）
cd src/native/netproxy
CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -trimpath -tags "$(cat release/DEFAULT_BUILD_TAGS_OTHERS)" -o netproxyctl ./cmd/netproxyctl

# 管理器 APK（需 JDK 25 + Android SDK 37）
cd src/android && ./gradlew :app:assembleDebug

# 模块 zip
sh .github/scripts/package-module.sh src/module <输出目录> <标准包名> <含管理器包名>
```

打包脚本要求 `src/module/NetProxy.apk` 存在（含管理器包用），标准包会排除它。
`src/module/bin/` 下的 `netproxyctl` 与 `sing-box` 是构建产物，不提交仓库。

---

## 与上游同步

```sh
git fetch upstream && git merge upstream/main
```

冲突高发区：`internal/module/app.go`（Prepare 流程）、`lifecycle.go`（sing-box 启动参数）、
`config.go`（运行时文件清单）、`MainActivity.kt`（分页构建）。

上游若也实现了分组功能，优先采用上游方案，本 fork 的 `internal/policy` 可整体移除。
