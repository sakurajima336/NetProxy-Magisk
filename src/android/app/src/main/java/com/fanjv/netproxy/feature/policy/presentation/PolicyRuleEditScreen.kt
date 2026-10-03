package com.fanjv.netproxy.feature.policy.presentation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.feature.policy.model.POLICY_ENTRY_KINDS
import com.fanjv.netproxy.feature.policy.model.PolicyEntry
import com.fanjv.netproxy.feature.policy.model.PolicyGroup
import com.fanjv.netproxy.feature.policy.model.policyEntryKindLabel
import com.fanjv.netproxy.feature.policy.model.policyEntryKindShort
import com.fanjv.netproxy.feature.policy.model.policyRuleNameError
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Switch
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextField
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.theme.MiuixTheme

/**
 * 规则组编辑页：给一组规则指定使用哪个节点组。
 *
 * 规则内容本身放在 rules/local/<名称>.list，由用户维护；
 * 这里只负责登记与归属，对应"规则组 -> 节点组"的关系。
 */
@Composable
internal fun PolicyRuleEditScreen(
    draft: PolicyRuleDraft,
    groups: List<PolicyGroup>,
    mutating: Boolean,
    onUpdate: ((PolicyRuleDraft) -> PolicyRuleDraft) -> Unit,
    onSave: () -> Unit,
    onDismiss: () -> Unit
) {
    val layoutDirection = LocalLayoutDirection.current
    val nameError = policyRuleNameError(draft.name)

    Scaffold(
        topBar = {
            TopAppBar(
                title = if (draft.isNew) "新建规则组" else "编辑规则组",
                navigationIcon = {
                    IconButton(onClick = onDismiss) {
                        Icon(Icons.Rounded.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    TextButton(
                        text = "保存",
                        onClick = onSave,
                        enabled = !mutating && nameError == null
                    )
                }
            )
        }
    ) { innerPadding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(
                start = innerPadding.calculateStartPadding(layoutDirection),
                end = innerPadding.calculateEndPadding(layoutDirection),
                top = innerPadding.calculateTopPadding(),
                bottom = innerPadding.calculateBottomPadding() + 24.dp,
            )
        ) {
            groupedCardSection(
                keyPrefix = "rule-name",
                title = { "名称" },
                items = listOf(
                    CardItem(key = "name") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            TextField(
                                value = draft.name,
                                onValueChange = { value ->
                                    onUpdate { it.copy(name = value, error = null) }
                                },
                                label = "规则组名",
                                modifier = Modifier.fillMaxWidth(),
                                // 改名会新建一份 .list，避免与旧文件混淆。
                                enabled = !mutating && draft.isNew
                            )
                            Text(
                                text = "对应 rules/local/${draft.name.ifBlank { "<名称>" }}.list",
                                style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                modifier = Modifier.padding(top = 6.dp)
                            )
                            val message = nameError ?: draft.error
                            if (message != null) {
                                Text(
                                    text = message,
                                    style = MiuixTheme.textStyles.footnote1,
                                    color = MiuixTheme.colorScheme.error,
                                    modifier = Modifier.padding(top = 6.dp)
                                )
                            }
                        }
                    }
                )
            )

            groupedCardSection(
                keyPrefix = "rule-group",
                title = { "使用哪个节点组" },
                items = listOf(
                    CardItem(key = "hint") {
                        Text(
                            text = "命中这个规则组的流量，会从所选节点组里出站。",
                            style = MiuixTheme.textStyles.footnote1,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                )
            )
            if (groups.isEmpty()) {
                item(key = "rule-group:empty") {
                    Card(modifier = Modifier.padding(horizontal = 14.dp, vertical = 6.dp)) {
                        Text(
                            text = "还没有节点组，请先到「节点组」分区新建一个。",
                            style = MiuixTheme.textStyles.footnote1,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }
            } else {
                groupedCardItems(
                    keyPrefix = "rule-group",
                    items = groups.map { group ->
                        CardItem(key = group.tag) {
                            BasicComponent(
                                title = group.tag,
                                summary = buildString {
                                    if (group.providers.isNotEmpty()) {
                                        append("订阅 ${group.providers.size} 个")
                                    }
                                    if (group.nodes.isNotEmpty()) {
                                        if (isNotEmpty()) append(" · ")
                                        append("单独节点 ${group.nodes.size} 个")
                                    }
                                    if (isEmpty()) append("未配置来源")
                                },
                                endActions = {
                                    Switch(
                                        checked = draft.group == group.tag,
                                        onCheckedChange = {
                                            onUpdate { it.copy(group = group.tag, error = null) }
                                        },
                                        enabled = !mutating
                                    )
                                }
                            )
                        }
                    }
                )
            }

            groupedCardSection(
                keyPrefix = "rule-enabled",
                title = { "状态" },
                items = listOf(
                    CardItem(key = "enabled") {
                        BasicComponent(
                            title = "启用",
                            summary = "停用后该规则组不参与路由",
                            endActions = {
                                Switch(
                                    checked = draft.enabled,
                                    onCheckedChange = { value ->
                                        onUpdate { it.copy(enabled = value) }
                                    },
                                    enabled = !mutating
                                )
                            }
                        )
                    }
                )
            )
        }
    }
}

/**
 * 规则组详情页：在这里直接增删域名/IP，不需要手写文件。
 *
 * 上方是输入区（选匹配方式 + 填内容 + 保存），下方是已保存列表，
 * 支持单条删除，也可进入多选模式批量删除。
 */
@Composable
internal fun PolicyRuleDetailScreen(
    detail: PolicyRuleDetailState,
    mutating: Boolean,
    onUpdate: ((PolicyRuleDetailState) -> PolicyRuleDetailState) -> Unit,
    onAdd: () -> Unit,
    onRemove: (PolicyEntry) -> Unit,
    onToggleSelect: (PolicyEntry) -> Unit,
    onRemoveSelected: () -> Unit,
    onDismiss: () -> Unit
) {
    val layoutDirection = LocalLayoutDirection.current

    Scaffold(
        topBar = {
            TopAppBar(
                title = detail.name,
                navigationIcon = {
                    IconButton(onClick = onDismiss) {
                        Icon(Icons.Rounded.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    if (detail.entries.isNotEmpty()) {
                        TextButton(
                            text = if (detail.selectionMode) "取消" else "多选",
                            onClick = { onUpdate { it.copy(selectionMode = !it.selectionMode) } }
                        )
                    }
                }
            )
        }
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(
                    start = innerPadding.calculateStartPadding(layoutDirection),
                    end = innerPadding.calculateEndPadding(layoutDirection),
                    top = innerPadding.calculateTopPadding(),
                )
        ) {
            LazyColumn(
                modifier = Modifier.weight(1f),
                contentPadding = PaddingValues(bottom = 12.dp)
            ) {
                // 输入区：选匹配方式 + 填域名/IP + 保存
                groupedCardSection(
                    keyPrefix = "entry-add",
                    title = { "添加域名或 IP" },
                    items = listOf(
                        CardItem(key = "input") {
                            Column(modifier = Modifier.padding(14.dp)) {
                                TextField(
                                    value = detail.draftValue,
                                    onValueChange = { value ->
                                        onUpdate { it.copy(draftValue = value, error = null) }
                                    },
                                    label = if (detail.draftKind == "ip") "例如 1.2.3.4 或 1.2.3.0/24"
                                    else "例如 openai.com",
                                    modifier = Modifier.fillMaxWidth(),
                                    enabled = !mutating
                                )
                                Text(
                                    text = "多个用逗号分隔，可一次添加",
                                    style = MiuixTheme.textStyles.footnote1,
                                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                    modifier = Modifier.padding(top = 6.dp)
                                )
                                if (detail.error != null) {
                                    Text(
                                        text = detail.error,
                                        style = MiuixTheme.textStyles.footnote1,
                                        color = MiuixTheme.colorScheme.error,
                                        modifier = Modifier.padding(top = 6.dp)
                                    )
                                }
                                TextButton(
                                    text = "保存",
                                    onClick = onAdd,
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(top = 10.dp),
                                    enabled = !mutating
                                )
                            }
                        }
                    )
                )

                // 匹配方式：用开关选择，选中即生效
                groupedCardSection(
                    keyPrefix = "entry-kind",
                    title = { "这个域名怎么匹配" },
                    items = listOf(
                        CardItem(key = "kind") {
                            Column {
                                POLICY_ENTRY_KINDS.forEach { kind ->
                                    BasicComponent(
                                        title = policyEntryKindLabel(kind),
                                        endActions = {
                                            Switch(
                                                checked = detail.draftKind == kind,
                                                onCheckedChange = { checked ->
                                                    if (checked) {
                                                        onUpdate { it.copy(draftKind = kind, error = null) }
                                                    }
                                                },
                                                enabled = !mutating
                                            )
                                        }
                                    )
                                }
                            }
                        }
                    )
                )

                // 已保存列表
                if (detail.entries.isEmpty()) {
                    item(key = "entries:empty") {
                        Card(modifier = Modifier.padding(horizontal = 14.dp, vertical = 6.dp)) {
                            Column(modifier = Modifier.padding(14.dp)) {
                                Text(text = "还没有添加任何域名或 IP")
                                Text(
                                    text = "在上面输入域名并保存，命中它的流量就会走这个规则组指定的节点组。",
                                    style = MiuixTheme.textStyles.footnote1,
                                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                    modifier = Modifier.padding(top = 6.dp)
                                )
                            }
                        }
                    }
                } else {
                    groupedCardSection(
                        keyPrefix = "entries",
                        title = { "已保存（${detail.entries.size}）" },
                        items = detail.entries.map { entry ->
                            CardItem(key = entry.key()) {
                                BasicComponent(
                                    title = entry.value,
                                    summary = policyEntryKindShort(entry.kind),
                                    endActions = {
                                        if (detail.selectionMode) {
                                            Switch(
                                                checked = entry.key() in detail.selected,
                                                onCheckedChange = { onToggleSelect(entry) }
                                            )
                                        } else {
                                            IconButton(
                                                onClick = { onRemove(entry) },
                                                enabled = !mutating
                                            ) {
                                                Icon(Icons.Rounded.Delete, contentDescription = "删除")
                                            }
                                        }
                                    }
                                )
                            }
                        }
                    )
                }

                if (detail.other > 0) {
                    item(key = "entries:other") {
                        Text(
                            text = "另有 ${detail.other} 条高级规则由文件维护，客户端不会改动。",
                            style = MiuixTheme.textStyles.footnote1,
                            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                            modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp)
                        )
                    }
                }
            }

            // 多选模式下底部出现批量删除
            if (detail.selectionMode) {
                BlurredBar(backdrop = null) {
                    Box(modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                        TextButton(
                            text = if (detail.selected.isEmpty()) "请选择要删除的条目"
                            else "删除选中的 ${detail.selected.size} 条",
                            onClick = onRemoveSelected,
                            modifier = Modifier.fillMaxWidth(),
                            enabled = detail.selected.isNotEmpty() && !mutating
                        )
                    }
                }
            }
        }
    }
}