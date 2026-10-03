package com.fanjv.netproxy.feature.policy.data

import com.fanjv.netproxy.core.command.NetProxyCtlClient
import com.fanjv.netproxy.feature.policy.model.PolicyEntryList
import com.fanjv.netproxy.feature.policy.model.PolicyEntryMutation
import com.fanjv.netproxy.feature.policy.model.PolicyGroup
import com.fanjv.netproxy.feature.policy.model.PolicyGroupList
import com.fanjv.netproxy.feature.policy.model.PolicyGroupWriteResult
import com.fanjv.netproxy.feature.policy.model.PolicyRuleDetail
import com.fanjv.netproxy.feature.policy.model.PolicyRuleFields
import com.fanjv.netproxy.feature.policy.model.PolicyRuleList
import com.fanjv.netproxy.feature.policy.model.PolicyRuleWriteResult
import kotlinx.serialization.json.decodeFromJsonElement

/** 规则组与节点组的数据层，通过 netproxyctl 的 schema=1 契约通信。 */
internal class PolicyRepository(
    private val client: NetProxyCtlClient
) {
    private val json = client.json

    /** 读取全部节点组。 */
    suspend fun listGroups(): PolicyGroupList =
        json.decodeFromJsonElement(client.execute("group", "list").data)

    /** 读取单个节点组。 */
    suspend fun showGroup(tag: String): PolicyGroup =
        json.decodeFromJsonElement(client.execute("group", "show", tag).data)

    /**
     * 新增或更新节点组。
     *
     * 只有非空参数才会下发给模块，未传入的字段保持原值，
     * 避免编辑一个字段时把其他字段清空。
     */
    suspend fun setGroup(
        tag: String,
        providers: List<String>? = null,
        nodes: List<String>? = null,
        include: String? = null,
        exclude: String? = null,
        strategy: String? = null,
        defaultNode: String? = null
    ): PolicyGroupWriteResult {
        val arguments = mutableListOf("group", "set", tag)
        // 显式传空列表表示清空，因此用 null 区分"未修改"。
        providers?.let { arguments += listOf("--providers", it.joinToString(",")) }
        nodes?.let { arguments += listOf("--nodes", it.joinToString(",")) }
        include?.takeIf(String::isNotBlank)?.let { arguments += listOf("--include", it) }
        exclude?.takeIf(String::isNotBlank)?.let { arguments += listOf("--exclude", it) }
        strategy?.takeIf(String::isNotBlank)?.let { arguments += listOf("--strategy", it) }
        defaultNode?.takeIf(String::isNotBlank)?.let { arguments += listOf("--default", it) }
        return json.decodeFromJsonElement(client.execute(*arguments.toTypedArray()).data)
    }

    /** 删除节点组。仍被规则组引用时模块会拒绝。 */
    suspend fun removeGroup(tag: String) {
        client.execute("group", "remove", tag)
    }

    /** 读取全部规则组。 */
    suspend fun listRules(): PolicyRuleList =
        json.decodeFromJsonElement(client.execute("rule", "list").data)

    /** 读取单个规则组的原始内容。 */
    suspend fun showRule(name: String): PolicyRuleDetail =
        json.decodeFromJsonElement(client.execute("rule", "show", name).data)

    /** 读取受支持的 .list 字段说明。 */
    suspend fun ruleFields(): PolicyRuleFields =
        json.decodeFromJsonElement(client.execute("rule", "fields").data)

    /** 新增或更新规则组。 */
    suspend fun setRule(
        name: String,
        group: String,
        enabled: Boolean? = null
    ): PolicyRuleWriteResult {
        val arguments = mutableListOf("rule", "set", name, "--group", group)
        enabled?.let { arguments += listOf("--enabled", it.toString()) }
        return json.decodeFromJsonElement(client.execute(*arguments.toTypedArray()).data)
    }

    /** 删除规则组。 */
    suspend fun removeRule(name: String) {
        client.execute("rule", "remove", name)
    }

    /** 读取规则组的域名/IP 条目。 */
    suspend fun listEntries(name: String): PolicyEntryList =
        json.decodeFromJsonElement(client.execute("rule", "entries", name).data)

    /** 添加域名/IP 条目。多个值用逗号分隔，模块侧会校验并去重。 */
    suspend fun addEntry(name: String, kind: String, value: String): PolicyEntryMutation =
        json.decodeFromJsonElement(
            client.execute("rule", "add", name, "--kind", kind, "--value", value).data
        )

    /** 删除域名/IP 条目。支持一次传多个值。 */
    suspend fun removeEntry(name: String, kind: String, value: String): PolicyEntryMutation =
        json.decodeFromJsonElement(
            client.execute("rule", "rm", name, "--kind", kind, "--value", value).data
        )
}