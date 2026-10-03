package com.fanjv.netproxy.feature.policy.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** 一个节点组：描述规则组可以使用哪些订阅与哪些单独节点。 */
@Serializable
internal data class PolicyGroup(
    /** 节点组名，同时是生成出的出站 tag。 */
    val tag: String,
    /** 整份订阅的 Catalog 分组 ID。 */
    val providers: List<String> = emptyList(),
    /** 单独挑选的节点，格式为 `<分组 ID>/<节点标签>`。 */
    val nodes: List<String> = emptyList(),
    /** 正则筛选，匹配节点名。 */
    val include: String = "",
    /** 正则排除，优先级高于 include。 */
    val exclude: String = "",
    /** 测速策略：urltest（自动延迟最低）或 manual（手动选择）。 */
    val strategy: String = "urltest",
    /** manual 策略下的默认选中节点。 */
    val default: String = "",
    /** 引用了该节点组的规则组名称。 */
    @SerialName("used_by")
    val usedBy: List<String> = emptyList()
)

/** 一个规则组：一组规则，指定走哪个节点组。 */
@Serializable
internal data class PolicyRule(
    val name: String,
    val group: String,
    val enabled: Boolean = true,
    /** 规则文件路径。 */
    val list: String = "",
    /** 规则文件是否存在。 */
    val exists: Boolean = false,
    /** 有效条目数。 */
    val count: Int = 0,
    /** 解析失败时的错误信息。 */
    val error: String = ""
)

/** 节点组列表响应。 */
@Serializable
internal data class PolicyGroupList(
    val groups: List<PolicyGroup> = emptyList(),
    val config: String = "",
    @SerialName("group_count")
    val groupCount: Int = 0
)

/** 规则组列表响应。 */
@Serializable
internal data class PolicyRuleList(
    val rules: List<PolicyRule> = emptyList(),
    @SerialName("list_dir")
    val listDir: String = "",
    val config: String = "",
    @SerialName("rule_count")
    val ruleCount: Int = 0
)

/** 规则组详情，包含原始文本与解析后的规则集。 */
@Serializable
internal data class PolicyRuleDetail(
    val name: String,
    val group: String,
    val enabled: Boolean = true,
    val path: String = "",
    val count: Int = 0,
    val content: String = ""
)

/** 节点组写入结果。 */
@Serializable
internal data class PolicyGroupWriteResult(
    val tag: String,
    val providers: List<String> = emptyList(),
    val nodes: List<String> = emptyList(),
    val strategy: String = "urltest",
    val created: Boolean = false
)

/** 规则组写入结果。 */
@Serializable
internal data class PolicyRuleWriteResult(
    val name: String,
    val group: String,
    val enabled: Boolean = true,
    val path: String = "",
    val created: Boolean = false
)

/** 受支持的 .list 字段说明。 */
@Serializable
internal data class PolicyRuleFields(
    val fields: List<String> = emptyList(),
    val sample: List<String> = emptyList(),
    val directory: String = ""
)

/** 节点组名称校验，与模块侧 policy 包保持一致。 */
internal val POLICY_GROUP_TAG_PATTERN = Regex("""^[\p{L}\p{N}][\p{L}\p{N}._ -]*$""")

/** 规则组名称不允许路径分隔符，避免越出规则目录。 */
internal fun policyRuleNameError(name: String): String? {
    val trimmed = name.trim()
    return when {
        trimmed.isEmpty() -> "规则组名不能为空"
        trimmed.contains('/') || trimmed.contains('\\') -> "规则组名不能包含路径分隔符"
        trimmed.contains("..") -> "规则组名不能包含 .."
        else -> null
    }
}

/** 节点组名称校验。 */
internal fun policyGroupTagError(tag: String): String? {
    val trimmed = tag.trim()
    return when {
        trimmed.isEmpty() -> "节点组名不能为空"
        !POLICY_GROUP_TAG_PATTERN.matches(trimmed) -> "节点组名含非法字符"
        else -> null
    }
}