import type { ReactNode } from "react";
import { StyleSheet, View, useWindowDimensions } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useFolds } from "../../modules/cadenza-fold";
import { splitPanes } from "../lib/fold";
import { useTheme } from "../theme";

/**
 * Panes shows the list and the detail side by side when there is room (or
 * when a foldable is half-opened like a book, split exactly on the hinge),
 * and otherwise just `single`.
 */
export function Panes(props: { list: ReactNode; detail: ReactNode; single: ReactNode }) {
  const { colors } = useTheme();
  const { width } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const folds = useFolds();
  const split = splitPanes(insets.left, width - insets.left - insets.right, folds);
  if (!split.twoPane) return <>{props.single}</>;
  return (
    <View style={styles.row}>
      <View style={[styles.list, { width: split.listWidth, borderRightColor: split.gap ? "transparent" : colors.line }]}>{props.list}</View>
      {split.gap ? <View style={{ width: split.gap }} accessibilityElementsHidden importantForAccessibility="no-hide-descendants" /> : null}
      <View style={styles.detail}>{props.detail}</View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flex: 1, flexDirection: "row" },
  list: { borderRightWidth: StyleSheet.hairlineWidth },
  detail: { flex: 1 },
});
