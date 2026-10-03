package com.fanjv.netproxy.feature.policy.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.ArrowBack
import androidx.compose.material.icons.rounded.Search
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.groupedCardItems
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.feature.catalog.presentation.nodes.CatalogNodesViewModel
import com.fanjv.netproxy.feature.policy.model.policyGroupTagError
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
 * 节点组编辑页。
 *
 * 一个节点组描述"可以用哪些节点"：
 *  - 勾选整份订阅（该订阅全部节点纳入）
 *  - 勾选单独节点（跨订阅挑选，用于便宜的好节点）
 *  - 正则筛选 / 排除，对上面两项的结果再过滤
 */
@Composable
internal fun PolicyGroupEditScreen(
    draft: PolicyGroupDraft,
    mutating: Boolean,
    onUpdate: ((PolicyGroupDraft) -> PolicyGroupDraft) -> Unit,
    onToggleProvider: (String) -> Unit,
    onToggleNode: (String) -> Unit,
    onSave: () -> Unit,
    onDismiss: () -> Unit,
    catalogViewModel: CatalogNodesViewModel = netProxyViewModel()
) {
    val catalogState by catalogViewModel.state.collectAsStateWithLifecycle()
    val layoutDirection = LocalLayoutDirection.current
    var keyword by remember { mutableStateOf("") }

    // 进入编辑页时加载节点目录，供勾选订阅与单独节点。
    LaunchedEffect(Unit) { catalogViewModel.refresh() }

    val groups = catalogState.groups
    val tagError = policyGroupTagError(draft.tag)

    Scaffold(
        topBar = {
            TopAppBar(
                title = if (draft.isNew) "新建节点组" else "编辑节点组",
                navigationIcon = {
                    IconButton(onClick = onDismiss) {
                        Icon(Icons.Rounded.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    TextButton(
                        text = "保存",
                        onClick = onSave,
                        enabled = !mutating && tagError == null
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
            // 名称
            groupedCardSection(
                keyPrefix = "group-name",
                title = { "名称" },
                items = listOf(
                    CardItem(key = "tag") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            TextField(
                                value = draft.tag,
                                onValueChange = { value ->
                                    onUpdate { it.copy(tag = value, error = null) }
                                },
                                label = "节点组名",
                                modifier = Modifier.fillMaxWidth(),
                                enabled = !mutating
                            )
                            val message = tagError ?: draft.error
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

            // 订阅：整份纳入
            groupedCardSection(
                keyPrefix = "group-providers",
                title = { "订阅（整份纳入）" },
                items = listOf(
                    CardItem(key = "provider-hint") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            Text(
                                text = "勾选后，该订阅的全部节点都会进入这个节点组。",
                                style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary
                            )
                        }
                    }
                )
            )
            if (groups.isEmpty()) {
                item(key = "providers:empty") {
                    PolicyInlineHint("还没有订阅，请先在订阅页添加。")
                }
            } else {
                groupedCardItems(
                    keyPrefix = "providers",
                    items = groups.map { entry ->
                        CardItem(key = entry.group.id) {
                            BasicComponent(
                                title = entry.group.name,
                                summary = "${entry.group.nodeCount} 个节点",
                                endActions = {
                                    Switch(
                                        checked = entry.group.id in draft.providers,
                                        onCheckedChange = { onToggleProvider(entry.group.id) },
                                        enabled = !mutating
                                    )
                                }
                            )
                        }
                    }
                )
            }

            // 单独节点：跨订阅挑选
            groupedCardSection(
                keyPrefix = "group-nodes",
                title = { "单独节点（已选 ${draft.nodes.size} 个）" },
                items = listOf(
                    CardItem(key = "node-search") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            Text(
                                text = "跨订阅挑选个别节点，例如便宜但速度好的那几个。",
                                style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary
                            )
                            TextField(
                                value = keyword,
                                onValueChange = { keyword = it },
                                label = "搜索节点",
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 10.dp),
                                leadingIcon = {
                                    Icon(Icons.Rounded.Search, contentDescription = null)
                                },
                                enabled = !mutating
                            )
                        }
                    }
                )
            )
            // LazyListScope 的 lambda 不是 @Composable 上下文，
            // 因此这里直接计算而不是用 remember。
            val matchedNodes = groups.flatMap { entry ->
                entry.nodes
                    .filter { node ->
                        keyword.isBlank() || node.tag.contains(keyword, ignoreCase = true)
                    }
                    .map { node -> entry.group.id to node.tag }
            }
            if (matchedNodes.isEmpty()) {
                item(key = "nodes:empty") {
                    PolicyInlineHint(
                        if (groups.isEmpty()) "还没有可选的节点。" else "没有匹配的节点。"
                    )
                }
            } else {
                // 只展示前若干条，避免超长列表拖慢滚动。
                groupedCardItems(
                    keyPrefix = "nodes",
                    items = matchedNodes.take(300).map { (groupId, tag) ->
                        val reference = "$groupId/$tag"
                        CardItem(key = reference) {
                            BasicComponent(
                                title = tag,
                                summary = groups.firstOrNull { it.group.id == groupId }?.group?.name.orEmpty(),
                                endActions = {
                                    Switch(
                                        checked = reference in draft.nodes,
                                        onCheckedChange = { onToggleNode(reference) },
                                        enabled = !mutating
                                    )
                                }
                            )
                        }
                    }
                )
                if (matchedNodes.size > 300) {
                    item(key = "nodes:truncated") {
                        PolicyInlineHint("仅显示前 300 个，请用搜索缩小范围。")
                    }
                }
            }

            // 正则筛选
            groupedCardSection(
                keyPrefix = "group-filter",
                title = { "正则筛选（可选）" },
                items = listOf(
                    CardItem(key = "filter") {
                        Column(modifier = Modifier.padding(14.dp)) {
                            TextField(
                                value = draft.include,
                                onValueChange = { value -> onUpdate { it.copy(include = value) } },
                                label = "只保留匹配的节点（include）",
                                modifier = Modifier.fillMaxWidth(),
                                enabled = !mutating
                            )
                            TextField(
                                value = draft.exclude,
                                onValueChange = { value -> onUpdate { it.copy(exclude = value) } },
                                label = "排除匹配的节点（exclude）",
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 10.dp),
                                enabled = !mutating
                            )
                            Text(
                                text = "留空表示不筛选。例如排除香港：^(?!.*(港|HK)).*$",
                                style = MiuixTheme.textStyles.footnote1,
                                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                                modifier = Modifier.padding(top = 8.dp)
                            )
                        }
                    }
                )
            )

            // 选择方式
            groupedCardSection(
                keyPrefix = "group-strategy",
                title = { "选择方式" },
                items = listOf(
                    CardItem(key = "strategy") {
                        Column {
                            BasicComponent(
                                title = "自动选择延迟最低",
                                summary = "组内节点自动测速，始终使用最快的",
                                endActions = {
                                    Switch(
                                        checked = draft.strategy == "urltest",
                                        onCheckedChange = {
                                            onUpdate { it.copy(strategy = "urltest") }
                                        },
                                        enabled = !mutating
                                    )
                                }
                            )
                            BasicComponent(
                                title = "手动选择",
                                summary = "由你在面板里指定使用哪个节点",
                                endActions = {
                                    Switch(
                                        checked = draft.strategy == "manual",
                                        onCheckedChange = {
                                            onUpdate { it.copy(strategy = "manual") }
                                        },
                                        enabled = !mutating
                                    )
                                }
                            )
                        }
                    }
                )
            )
        }
    }
}

@Composable
private fun PolicyInlineHint(text: String) {
    Card(modifier = Modifier.padding(horizontal = 14.dp, vertical = 6.dp)) {
        Text(
            text = text,
            style = MiuixTheme.textStyles.footnote1,
            color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
            modifier = Modifier.padding(14.dp)
        )
    }
}