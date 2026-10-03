package com.fanjv.netproxy.feature.policy.presentation

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.fanjv.netproxy.feature.policy.data.PolicyRepository
import com.fanjv.netproxy.feature.policy.model.PolicyEntry
import com.fanjv.netproxy.feature.policy.model.PolicyGroup
import com.fanjv.netproxy.feature.policy.model.PolicyRule
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 分组页的顶层分区：节点组与规则组。 */
internal enum class PolicySection {
    Groups,
    Rules
}

/** 分组页状态。 */
internal data class PolicyUiState(
    val loading: Boolean = true,
    val section: PolicySection = PolicySection.Groups,
    val groups: List<PolicyGroup> = emptyList(),
    val rules: List<PolicyRule> = emptyList(),
    val error: String? = null,
    /** 正在进行写操作，用于禁用按钮避免重复提交。 */
    val mutating: Boolean = false,
    /** 节点组编辑草稿，非空时展示编辑页。 */
    val groupDraft: PolicyGroupDraft? = null,
    /** 规则组编辑草稿，非空时展示编辑页。 */
    val ruleDraft: PolicyRuleDraft? = null,
    /** 规则组内容查看，非空时展示详情页。 */
    val ruleDetail: PolicyRuleDetailState? = null
)

/** 节点组编辑草稿。 */
internal data class PolicyGroupDraft(
    val original: PolicyGroup? = null,
    val tag: String = "",
    val providers: Set<String> = emptySet(),
    val nodes: Set<String> = emptySet(),
    val include: String = "",
    val exclude: String = "",
    val strategy: String = "urltest",
    val defaultNode: String = "",
    val error: String? = null
) {
    val isNew: Boolean get() = original == null
}

/** 规则组编辑草稿。 */
internal data class PolicyRuleDraft(
    val original: PolicyRule? = null,
    val name: String = "",
    val group: String = "",
    val enabled: Boolean = true,
    val error: String? = null
) {
    val isNew: Boolean get() = original == null
}

/** 规则组详情状态：展示域名/IP 条目，并支持增删。 */
internal data class PolicyRuleDetailState(
    val name: String,
    val group: String,
    /** 已保存的条目。 */
    val entries: List<PolicyEntry> = emptyList(),
    /** 由其他方式维护的条目数（如手写的 process_name），只提示不展示。 */
    val other: Int = 0,
    /** 正在输入的新条目。 */
    val draftKind: String = "suffix",
    val draftValue: String = "",
    /** 多选删除时被勾选的条目键。 */
    val selected: Set<String> = emptySet(),
    /** 是否处于多选模式。 */
    val selectionMode: Boolean = false,
    val error: String? = null
)

/** 条目在集合中的唯一键。 */
internal fun PolicyEntry.key(): String = "$kind\u0000$value"

/** 分组页 ViewModel：节点组与规则组的读写入口。 */
internal class PolicyViewModel(
    private val repository: PolicyRepository
) : ViewModel() {

    private val _state = MutableStateFlow(PolicyUiState())
    val state: StateFlow<PolicyUiState> = _state.asStateFlow()

    /** 读取全部节点组与规则组。 */
    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            runCatching {
                val groups = repository.listGroups().groups
                val rules = repository.listRules().rules
                groups to rules
            }.onSuccess { (groups, rules) ->
                _state.update {
                    it.copy(loading = false, groups = groups, rules = rules, error = null)
                }
            }.onFailure { error ->
                _state.update { it.copy(loading = false, error = error.message ?: "读取分组失败") }
            }
        }
    }

    fun selectSection(section: PolicySection) {
        _state.update { it.copy(section = section) }
    }

    /** 新建节点组草稿。 */
    fun startNewGroup() {
        _state.update { it.copy(groupDraft = PolicyGroupDraft()) }
    }

    /** 编辑已有节点组。 */
    fun editGroup(group: PolicyGroup) {
        _state.update {
            it.copy(
                groupDraft = PolicyGroupDraft(
                    original = group,
                    tag = group.tag,
                    providers = group.providers.toSet(),
                    nodes = group.nodes.toSet(),
                    include = group.include,
                    exclude = group.exclude,
                    strategy = group.strategy,
                    defaultNode = group.default
                )
            )
        }
    }

    fun dismissGroupDraft() {
        _state.update { it.copy(groupDraft = null) }
    }

    fun updateGroupDraft(transform: (PolicyGroupDraft) -> PolicyGroupDraft) {
        _state.update { current ->
            current.groupDraft?.let { current.copy(groupDraft = transform(it)) } ?: current
        }
    }

    fun toggleProvider(providerId: String) {
        updateGroupDraft { draft ->
            val providers = draft.providers.toMutableSet()
            if (!providers.add(providerId)) providers.remove(providerId)
            draft.copy(providers = providers)
        }
    }

    fun toggleNode(reference: String) {
        updateGroupDraft { draft ->
            val nodes = draft.nodes.toMutableSet()
            if (!nodes.add(reference)) nodes.remove(reference)
            draft.copy(nodes = nodes)
        }
    }

    /** 保存节点组草稿。校验通过后写入并重新读取。 */
    fun saveGroupDraft() {
        val draft = _state.value.groupDraft ?: return
        val tagError = com.fanjv.netproxy.feature.policy.model.policyGroupTagError(draft.tag)
        if (tagError != null) {
            updateGroupDraft { it.copy(error = tagError) }
            return
        }
        if (draft.strategy == "manual" && draft.defaultNode.isBlank()) {
            updateGroupDraft { it.copy(error = "手动选择模式下需要指定默认节点") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(mutating = true) }
            runCatching {
                repository.setGroup(
                    tag = draft.tag.trim(),
                    providers = draft.providers.toList(),
                    nodes = draft.nodes.toList(),
                    include = draft.include.trim(),
                    exclude = draft.exclude.trim(),
                    strategy = draft.strategy,
                    defaultNode = draft.defaultNode.trim()
                )
            }.onSuccess {
                _state.update { it.copy(mutating = false, groupDraft = null) }
                refresh()
            }.onFailure { error ->
                _state.update { it.copy(mutating = false) }
                updateGroupDraft { it.copy(error = error.message ?: "保存节点组失败") }
            }
        }
    }

    /** 删除节点组。被规则组引用时模块会拒绝并返回原因。 */
    fun removeGroup(tag: String) {
        viewModelScope.launch {
            _state.update { it.copy(mutating = true, error = null) }
            runCatching { repository.removeGroup(tag) }
                .onSuccess {
                    _state.update { it.copy(mutating = false) }
                    refresh()
                }
                .onFailure { error ->
                    _state.update { it.copy(mutating = false, error = error.message ?: "删除节点组失败") }
                }
        }
    }

    /** 新建规则组草稿。 */
    fun startNewRule() {
        _state.update {
            it.copy(
                ruleDraft = PolicyRuleDraft(
                    group = it.groups.firstOrNull()?.tag.orEmpty()
                )
            )
        }
    }

    /** 编辑已有规则组。 */
    fun editRule(rule: PolicyRule) {
        _state.update {
            it.copy(
                ruleDraft = PolicyRuleDraft(
                    original = rule,
                    name = rule.name,
                    group = rule.group,
                    enabled = rule.enabled
                )
            )
        }
    }

    fun dismissRuleDraft() {
        _state.update { it.copy(ruleDraft = null) }
    }

    fun updateRuleDraft(transform: (PolicyRuleDraft) -> PolicyRuleDraft) {
        _state.update { current ->
            current.ruleDraft?.let { current.copy(ruleDraft = transform(it)) } ?: current
        }
    }

    /** 保存规则组草稿。 */
    fun saveRuleDraft() {
        val draft = _state.value.ruleDraft ?: return
        val nameError = com.fanjv.netproxy.feature.policy.model.policyRuleNameError(draft.name)
        if (nameError != null) {
            updateRuleDraft { it.copy(error = nameError) }
            return
        }
        if (draft.group.isBlank()) {
            updateRuleDraft { it.copy(error = "请先选择节点组") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(mutating = true) }
            runCatching {
                repository.setRule(
                    name = draft.name.trim(),
                    group = draft.group,
                    enabled = draft.enabled
                )
            }.onSuccess {
                _state.update { it.copy(mutating = false, ruleDraft = null) }
                refresh()
            }.onFailure { error ->
                _state.update { it.copy(mutating = false) }
                updateRuleDraft { it.copy(error = error.message ?: "保存规则组失败") }
            }
        }
    }

    /** 删除规则组。 */
    fun removeRule(name: String) {
        viewModelScope.launch {
            _state.update { it.copy(mutating = true, error = null) }
            runCatching { repository.removeRule(name) }
                .onSuccess {
                    _state.update { it.copy(mutating = false) }
                    refresh()
                }
                .onFailure { error ->
                    _state.update { it.copy(mutating = false, error = error.message ?: "删除规则组失败") }
                }
        }
    }

    /** 打开规则组详情，加载域名/IP 条目。 */
    fun openRuleDetail(name: String) {
        viewModelScope.launch {
            runCatching {
                val list = repository.listEntries(name)
                PolicyRuleDetailState(
                    name = list.name,
                    group = list.group,
                    entries = list.entries,
                    other = list.other
                )
            }
                .onSuccess { detail -> _state.update { it.copy(ruleDetail = detail) } }
                .onFailure { error ->
                    _state.update { it.copy(error = error.message ?: "读取规则条目失败") }
                }
        }
    }

    fun dismissRuleDetail() {
        _state.update { it.copy(ruleDetail = null) }
    }

    /** 更新详情页的编辑状态（输入框、匹配方式、多选）。 */
    fun updateRuleDetail(transform: (PolicyRuleDetailState) -> PolicyRuleDetailState) {
        _state.update { current ->
            current.ruleDetail?.let { current.copy(ruleDetail = transform(it)) } ?: current
        }
    }

    /** 切换某条目的勾选状态。 */
    fun toggleEntrySelection(entry: PolicyEntry) {
        updateRuleDetail { detail ->
            val selected = detail.selected.toMutableSet()
            if (!selected.add(entry.key())) selected.remove(entry.key())
            detail.copy(selected = selected)
        }
    }

    /** 进入或退出多选模式。退出时清空勾选。 */
    fun setSelectionMode(enabled: Boolean) {
        updateRuleDetail { it.copy(selectionMode = enabled, selected = if (enabled) it.selected else emptySet()) }
    }

    /** 添加当前输入框中的条目。 */
    fun addEntry() {
        val detail = _state.value.ruleDetail ?: return
        val value = detail.draftValue.trim()
        if (value.isEmpty()) {
            updateRuleDetail { it.copy(error = "请输入域名或 IP") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(mutating = true) }
            runCatching { repository.addEntry(detail.name, detail.draftKind, value) }
                .onSuccess { result ->
                    _state.update { it.copy(mutating = false) }
                    updateRuleDetail {
                        it.copy(
                            entries = result.entries,
                            draftValue = "",
                            error = null
                        )
                    }
                }
                .onFailure { error ->
                    _state.update { it.copy(mutating = false) }
                    updateRuleDetail { it.copy(error = error.message ?: "添加失败") }
                }
        }
    }

    /** 删除单条条目。 */
    fun removeEntry(entry: PolicyEntry) {
        val detail = _state.value.ruleDetail ?: return
        viewModelScope.launch {
            _state.update { it.copy(mutating = true) }
            runCatching { repository.removeEntry(detail.name, entry.kind, entry.value) }
                .onSuccess { result ->
                    _state.update { it.copy(mutating = false) }
                    updateRuleDetail {
                        it.copy(
                            entries = result.entries,
                            selected = it.selected - entry.key(),
                            error = null
                        )
                    }
                }
                .onFailure { error ->
                    _state.update { it.copy(mutating = false) }
                    updateRuleDetail { it.copy(error = error.message ?: "删除失败") }
                }
        }
    }

    /** 多选删除：把勾选的条目一次提交给模块。 */
    fun removeSelectedEntries() {
        val detail = _state.value.ruleDetail ?: return
        val targets = detail.entries.filter { it.key() in detail.selected }
        if (targets.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(mutating = true) }
            var lastError: String? = null
            var entries = detail.entries
            // 模块按 (类型, 值) 删除，这里按类型分组一次传多个值，减少调用次数。
            targets.groupBy { it.kind }.forEach { (kind, group) ->
                runCatching {
                    repository.removeEntry(detail.name, kind, group.joinToString(",") { it.value })
                }
                    .onSuccess { result -> entries = result.entries }
                    .onFailure { error -> lastError = error.message ?: "删除失败" }
            }
            _state.update { it.copy(mutating = false) }
            updateRuleDetail {
                it.copy(
                    entries = entries,
                    selected = emptySet(),
                    selectionMode = false,
                    error = lastError
                )
            }
        }
    }

    fun dismissError() {
        _state.update { it.copy(error = null) }
    }
}