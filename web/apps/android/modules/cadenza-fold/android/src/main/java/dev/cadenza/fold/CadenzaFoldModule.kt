package dev.cadenza.fold

import android.app.Activity
import androidx.core.content.ContextCompat
import androidx.core.util.Consumer
import androidx.window.java.layout.WindowInfoTrackerCallbackAdapter
import androidx.window.layout.FoldingFeature
import androidx.window.layout.WindowInfoTracker
import androidx.window.layout.WindowLayoutInfo
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition

/**
 * Reports a foldable's hinge to JavaScript, so the app can split its two
 * panes along the fold instead of drawing content across it.
 *
 * Each fold is sent with its bounds in window pixels (JavaScript converts to
 * dp), its orientation, its state (flat or half-opened) and whether it
 * separates the window into two areas. Phones and tablets report no folds.
 */
class CadenzaFoldModule : Module() {
  private var adapter: WindowInfoTrackerCallbackAdapter? = null
  private var listening: Activity? = null
  private var last: Map<String, Any> = mapOf("folds" to emptyList<Any>())

  private val consumer = Consumer<WindowLayoutInfo> { info ->
    last = mapOf("folds" to info.displayFeatures.filterIsInstance<FoldingFeature>().map(::toMap))
    sendEvent("onLayoutChange", last)
  }

  override fun definition() = ModuleDefinition {
    Name("CadenzaFold")
    Events("onLayoutChange")

    // The latest layout, for the first render before any event arrives.
    Function("current") { last }

    OnStartObserving { start() }
    OnStopObserving { stop() }
    OnActivityEntersForeground { start() }
    OnActivityEntersBackground { stop() }
    OnDestroy { stop() }
  }

  private fun start() {
    val activity = appContext.currentActivity ?: return
    if (listening === activity) return
    stop()
    val a = WindowInfoTrackerCallbackAdapter(WindowInfoTracker.getOrCreate(activity))
    a.addWindowLayoutInfoListener(activity, ContextCompat.getMainExecutor(activity), consumer)
    adapter = a
    listening = activity
  }

  private fun stop() {
    adapter?.removeWindowLayoutInfoListener(consumer)
    adapter = null
    listening = null
  }

  private fun toMap(f: FoldingFeature): Map<String, Any> = mapOf(
    "left" to f.bounds.left,
    "top" to f.bounds.top,
    "right" to f.bounds.right,
    "bottom" to f.bounds.bottom,
    "orientation" to if (f.orientation == FoldingFeature.Orientation.VERTICAL) "vertical" else "horizontal",
    "state" to if (f.state == FoldingFeature.State.HALF_OPENED) "half_opened" else "flat",
    "separating" to f.isSeparating,
    "occluding" to (f.occlusionType == FoldingFeature.OcclusionType.FULL),
  )
}
