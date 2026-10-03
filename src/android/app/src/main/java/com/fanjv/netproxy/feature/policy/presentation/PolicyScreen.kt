package com.fanjv.netproxy.feature.policy.presentation

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.add
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fanjv.netproxy.core.di.netProxyViewModel
import com.fanjv.netproxy.core.ui.component.AppSnackbarHost
import com.fanjv.netproxy.core.ui.component.BlurredBar
import com.fanjv.netproxy.core.ui.component.CardItem
import com.fanjv.netproxy.core.ui.component.SnackbarNoticeEffect
import com.fanjv.netproxy.core.ui.component.groupedCardSection
import com.fanjv.netproxy.core.ui.component.rememberAppSnackbarHostState
import com.fanjv.netproxy.core.ui.component.rememberBlurBackdrop
import com.fanjv.netproxy.feature.policy.model.PolicyGroup
import com.fanjv.netproxy.feature.policy.model.PolicyRule
import top.yukonga.miuix.kmp.basic.BasicComponent
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.Icon
import top.yukonga.miuix.kmp.basic.IconButton
import top.yukonga.miuix.kmp.basic.InfiniteProgressIndicator
import top.yukonga.miuix.kmp.basic.MiuixScrollBehavior
import top.yukonga.miuix.kmp.basic.Scaffold
import top.yukonga.miuix.kmp.basic.ScrollBehavior
import top.yukonga.miuix.kmp.basic.TabRow
import top.yukonga.miuix.kmp.basic.Text
import top.yukonga.miuix.kmp.basic.TextButton
import top.yukonga.miuix.kmp.basic.TopAppBar
import top.yukonga.miuix.kmp.blur.layerBackdrop
import top.yukonga.miuix.kmp.theme.MiuixTheme

/**
 * 分组页：上层是节点组（用哪些订阅与节点），下层是规则组（一组规则走哪个节点组）。
 *
 * 规则组展开后可查看规则内容；节点组可勾选整份订阅、单独节点并设置正则筛选。
 */
@Composable
internal fun PolicyScreen(
    bottomPadding: androidx.compose.ui.unit.Dp,
    isActive: Boolean,
    viewModel: PolicyViewModel = netProxyViewModel()
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbarHostState = rememberAppSnackbarHostState()
    val blurBackdrop = rememberBlurBackdrop()

    // 首次进入或从后台返回时刷新，保持与模块侧一致。
    LaunchedEffect(isActive) {
        if (isActive) viewModel.refresh()
    }

    SnackbarNoticeEffect(
        eventId = state.error?.hashCode()?.toLong() ?: 0L,
        message = state.error.orEmpty(),
        isError = true,
        hostState = snackbarHostState,
        onConsumed = viewModel::dismissError
    )

    // 编辑与详情作为覆盖层，避免打断列表滚动位置。
    state.groupDraft?.let { draft ->
        PolicyGroupEditScreen(
            draft = draft,
            mutating = state.mutating,
            onUpdate = viewModel::updateGroupDraft,
            onToggleProvider = viewModel::toggleProvider,
            onToggleNode = viewModel::toggleNode,
            onSave = viewModel::saveGroupDraft,
            onDismiss = viewModel::dismissGroupDraft
        )
        return
    }
    state.ruleDraft?.let { draft ->
        PolicyRuleEditScreen(
            draft = draft,
            groups = state.groups,
            mutating = state.mutating,
            onUpdate = viewModel::updateRuleDraft,
            onSave = viewModel::saveRuleDraft,
            onDismiss = viewModel::dismissRuleDraft
        )
        return
    }
    state.ruleDetail?.let { detail ->
        PolicyRuleDetailScreen(detail = detail, onDismiss = viewModel::dismissRuleDetail)
        return
    }

    val layoutDirection = LocalLayoutDirection.current
    val scrollBehavior = MiuixScrollBehavior()

    Scaffold(
        topBar = {
            TopAppBar(
                title = "分组",
                scrollBehavior = scrollBehavior,
                navigationIcon = {},
                actions = {
                    IconButton(onClick = viewModel::refresh) {
                        Icon(Icons.Rounded.Refresh, contentDescription = "刷新")
                    }
                }
            )
        },
        bottomBar = {
            BlurredBar(backdrop = blurBackdrop) {
                PolicySectionTabs(
                    section = state.section,
                    onSelect = viewModel::selectSection
                )
            }
        }
    ) { innerPadding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .then(
                    if (blurBackdrop != null) Modifier.layerBackdrop(blurBackdrop) else Modifier
                )
        ) {
            if (state.loading && state.groups.isEmpty() && state.rules.isEmpty()) {
                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    InfiniteProgressIndicator()
                }
                return@Box
            }

            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(
                    start = innerPadding.calculateStartPadding(layoutDirection),
                    end = innerPadding.calculateEndPadding(layoutDirection),
                    top = innerPadding.calculateTopPadding(),
                    bottom = innerPadding.calculateBottomPadding() + bottomPadding,
                ),
                verticalArrangement = Arrangement.spacedBy(0.dp)
            ) {
                when (state.section) {
                    PolicySection.Groups -> groupsSection(
                        groups = state.groups,
                        mutating = state.mutating,
                        onEdit = viewModel::editGroup,
                        onRemove = viewModel::removeGroup,
                        onAdd = viewModel::startNewGroup
                    )

                    PolicySection.Rules -> rulesSection(
                        rules = state.rules,
                        mutating = state.mutating,
                        onEdit = viewModel::editRule,
                        onRemove = viewModel::removeRule,
                        onOpen = viewModel::openRuleDetail,
                        onAdd = viewModel::startNewRule
                    )
                }
            }
        }
    }
}

@Composable
private fun PolicySectionTabs(
    section: PolicySection,
    onSelect: (PolicySection) -> Unit
) {
    val tabs = listOf(
        PolicySection.Groups to "节点组",
        PolicySection.Rules to "规则组"
    )
    TabRow(
        modifier = Modifier.fillMaxWidth(),
        tabs = tabs.map { it.second },
        selectedTabIndex = tabs.indexOfFirst { it.first == section }.coerceAtLeast(0),
        onTabSelected = { index -> tabs.getOrNull(index)?.let { onSelect(it.first) } }
    )
}

private fun androidx.compose.foundation.lazy.LazyListScope.groupsSection(
    groups: List<PolicyGroup>,
    mutating: Boolean,
    onEdit: (PolicyGroup) -> Unit,
    onRemove: (String) -> Unit,
    onAdd: () -> Unit
) {
    if (groups.isEmpty()) {
        item(key = "groups:empty") {
            PolicyEmptyHint(
                title = "还没有节点组",
                description = "节点组用来描述一组节点：可以整份订阅，也可以单独挑几个节点。"
            )
        }
    } else {
        groupedCardSection(
            keyPrefix = "groups",
            title = { "节点组（${groups.size}）" },
            items = groups.map { group ->
                CardItem(key = group.tag) {
                    PolicyGroupRow(
                        group = group,
                        mutating = mutating,
                        onEdit = { onEdit(group) },
                        onRemove = { onRemove(group.tag) }
                    )
                }
            }
        )
    }
    item(key = "groups:add") {
        AddRow(text = "新建节点组", onClick = onAdd)
    }
}

private fun androidx.compose.foundation.lazy.LazyListScope.rulesSection(
    rules: List<PolicyRule>,
    mutating: Boolean,
    onEdit: (PolicyRule) -> Unit,
    onRemove: (String) -> Unit,
    onOpen: (String) -> Unit,
    onAdd: () -> Unit
) {
    if (rules.isEmpty()) {
        item(key = "rules:empty") {
            PolicyEmptyHint(
                title = "还没有规则组",
                description = "规则组对应 rules/local 下的 .list 文件，指定它使用哪个节点组。"
            )
        }
    } else {
        groupedCardSection(
            keyPrefix = "rules",
            title = { "规则组（${rules.size}）" },
            items = rules.map { rule ->
                CardItem(key = rule.name) {
                    PolicyRuleRow(
                        rule = rule,
                        mutating = mutating,
                        onOpen = { onOpen(rule.name) },
                        onEdit = { onEdit(rule) },
                        onRemove = { onRemove(rule.name) }
                    )
                }
            }
        )
    }
    item(key = "rules:add") {
        AddRow(text = "新建规则组", onClick = onAdd)
    }
}

@Composable
private fun PolicyGroupRow(
    group: PolicyGroup,
    mutating: Boolean,
    onEdit: () -> Unit,
    onRemove: () -> Unit
) {
    val summary = buildString {
        if (group.providers.isNotEmpty()) append("订阅 ${group.providers.size} 个")
        if (group.nodes.isNotEmpty()) {
            if (isNotEmpty()) append(" · ")
            append("单独节点 ${group.nodes.size} 个")
        }
        if (isEmpty()) append("未配置来源")
    }
    BasicComponent(
        title = group.tag,
        summary = summary,
        endActions = {
            if (group.usedBy.isNotEmpty()) {
                Text(
                    text = "被 ${group.usedBy.size} 个规则引用",
                    style = MiuixTheme.textStyles.footnote1,
                    color = MiuixTheme.colorScheme.onSurfaceVariantSummary
                )
            }
            IconButton(onClick = onEdit, enabled = !mutating) {
                Icon(Icons.Rounded.Edit, contentDescription = "编辑")
            }
            IconButton(onClick = onRemove, enabled = !mutating) {
                Icon(Icons.Rounded.Delete, contentDescription = "删除")
            }
        }
    )
}

@Composable
private fun PolicyRuleRow(
    rule: PolicyRule,
    mutating: Boolean,
    onOpen: () -> Unit,
    onEdit: () -> Unit,
    onRemove: () -> Unit
) {
    val summary = buildString {
        append("节点组：${rule.group}")
        when {
            rule.error.isNotEmpty() -> append(" · 解析失败")
            !rule.exists -> append(" · 缺少 .list 文件")
            else -> append(" · ${rule.count} 条规则")
        }
        if (!rule.enabled) append(" · 已停用")
    }
    BasicComponent(
        title = rule.name,
        summary = summary,
        onClick = onOpen,
        endActions = {
            IconButton(onClick = onEdit, enabled = !mutating) {
                Icon(Icons.Rounded.Edit, contentDescription = "编辑")
            }
            IconButton(onClick = onRemove, enabled = !mutating) {
                Icon(Icons.Rounded.Delete, contentDescription = "删除")
            }
        }
    )
}

@Composable
private fun AddRow(text: String, onClick: () -> Unit) {
    Box(modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
        TextButton(
            text = text,
            onClick = onClick,
            modifier = Modifier.fillMaxWidth()
        )
    }
}

@Composable
private fun PolicyEmptyHint(title: String, description: String) {
    Card(modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
        Column(modifier = Modifier.padding(16.dp)) {
            Text(text = title, fontWeight = FontWeight.Medium)
            Text(
                text = description,
                style = MiuixTheme.textStyles.footnote1,
                color = MiuixTheme.colorScheme.onSurfaceVariantSummary,
                modifier = Modifier.padding(top = 6.dp)
            )
        }
    }
}

/** 供外部复用的省略文本，避免超长节点名撑破布局。 */
@Composable
internal fun PolicyEllipsisText(text: String, modifier: Modifier = Modifier) {
    Text(
        text = text,
        modifier = modifier,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis
    )
}