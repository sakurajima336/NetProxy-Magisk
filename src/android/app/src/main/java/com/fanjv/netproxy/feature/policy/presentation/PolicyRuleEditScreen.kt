package com.fanjv.netproxy.feature.policy.presentation

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
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.feature.policy.model.PolicyGroup
import com.fanjv.netproxy.feature.policy.model.policyRuleNameError
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.Switch
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
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
 * 规则组内容查看页：展示 .list 原文与条目数。
 *
 * 采用只读展示，避免在缺少语法高亮与校验的情况下误改规则文件。
 */
@Composable
internal fun PolicyRuleDetailScreen(
    detail: PolicyRuleDetailState,
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
                keyPrefix = "detail",
                title = { "概览" },
                items = listOf(
                    CardItem(key = "overview") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            Text(text = "节点组：${detail.group}")
                            Text(
                                text = "${detail.count} 条规则",
                                style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                modifier = Modifier.padding(top = 4.dp)
                            )
                        }
                    }
                )
            )
            groupedCardSection(
                keyPrefix = "detail-content",
                title = { "规则内容" },
                items = listOf(
                    CardItem(key = "content") {
                        Text(
                            text = detail.content.ifBlank { "（空）" },
                            style = MiuixTheme.textStyles.footnote1,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                )
            )
        }
    }
}