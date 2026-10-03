package com.fanjv.netproxy.navigation

import androidx.compose.animation.core.EaseInOut
import androidx.compose.animation.core.tween
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.foundation.pager.PagerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.AccountTree
import androidx.compose.material.icons.rounded.CloudSync
import androidx.compose.material.icons.rounded.Dashboard
import androidx.compose.material.icons.rounded.Router
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.graphics.vector.ImageVector
import com.fanjv.netproxy.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlin.math.abs

@Composable
internal fun rememberMainPagerState(
    pagerState: PagerState,
    coroutineScope: CoroutineScope = rememberCoroutineScope()
): MainPagerState = rememberSaveable(
    pagerState,
    coroutineScope,
    saver = MainPagerState.saver(pagerState, coroutineScope)
) {
    MainPagerState(pagerState, coroutineScope)
}

internal class MainPagerState(
    val pagerState: PagerState,
    private val coroutineScope: CoroutineScope
) {
    var selectedPage by mutableIntStateOf(pagerState.currentPage)
        internal set

    var isNavigating by mutableStateOf(false)
        private set

    private var navigationJob: Job? = null

    fun animateToPage(targetIndex: Int) {
        if (targetIndex == selectedPage) return

        navigationJob?.cancel()
        selectedPage = targetIndex
        isNavigating = true

        val distance = abs(targetIndex - pagerState.currentPage).coerceAtLeast(2)
        val duration = 100 * distance + 100
        val layoutInfo = pagerState.layoutInfo
        val pageSize = layoutInfo.pageSize + layoutInfo.pageSpacing
        val currentDistanceInPages =
            targetIndex - pagerState.currentPage - pagerState.currentPageOffsetFraction
        val scrollPixels = currentDistanceInPages * pageSize

        navigationJob = coroutineScope.launch {
            val currentJob = coroutineContext.job
            try {
                pagerState.animateScrollBy(
                    value = scrollPixels,
                    animationSpec = tween(easing = EaseInOut, durationMillis = duration)
                )
            } finally {
                if (navigationJob == currentJob) {
                    isNavigating = false
                    if (pagerState.currentPage != targetIndex) {
                        selectedPage = pagerState.currentPage
                    }
                }
            }
        }
    }

    fun syncPage() {
        if (!isNavigating && selectedPage != pagerState.currentPage) {
            selectedPage = pagerState.currentPage
        }
    }

    companion object {
        fun saver(
            pagerState: PagerState,
            coroutineScope: CoroutineScope
        ): Saver<MainPagerState, Int> = Saver(
            save = { it.selectedPage },
            restore = { savedPage ->
                MainPagerState(pagerState, coroutineScope).apply {
                    selectedPage = savedPage
                }
            }
        )
    }
}

internal enum class AppDestination(
    val labelRes: Int,
    val icon: ImageVector,
) {
    Dashboard(R.string.dashboard, Icons.Rounded.Dashboard),
    Nodes(R.string.nodes, Icons.Rounded.Router),
    Subscriptions(R.string.subscriptions, Icons.Rounded.CloudSync),
    Settings(R.string.settings, Icons.Rounded.Settings),

    /**
     * 分组页：节点组与规则组管理。
     *
     * 声明在最后，但显示位置由 MainActivity 插到 Nodes 之前。
     * 是否显示由设置开关控制；功能稳定后移除开关即可成为默认入口。
     */
    Policy(R.string.policy, Icons.Rounded.AccountTree),
}

/** 分组页开关的偏好键。 */
internal const val POLICY_TAB_ENABLED_KEY = "policy_tab_enabled"

/** 默认不显示分组页，需在设置中显式开启。 */
internal const val POLICY_TAB_ENABLED_DEFAULT = false
