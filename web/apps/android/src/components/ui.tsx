import type { ReactNode } from "react";
import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View, type TextInputProps } from "react-native";
import { useTheme } from "../theme";

export function Button(props: { title: string; onPress: () => void; kind?: "primary" | "plain" | "danger"; disabled?: boolean; busy?: boolean }) {
  const { colors } = useTheme();
  const kind = props.kind ?? "plain";
  const bg = kind === "primary" ? colors.accent : "transparent";
  const fg = kind === "primary" ? colors.accentText : kind === "danger" ? colors.danger : colors.accent;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!props.disabled, busy: !!props.busy }}
      disabled={props.disabled || props.busy}
      onPress={props.onPress}
      style={({ pressed }) => [styles.button, { backgroundColor: bg, borderColor: kind === "primary" ? bg : colors.line, opacity: props.disabled ? 0.5 : pressed ? 0.7 : 1 }]}
    >
      {props.busy ? <ActivityIndicator color={fg} /> : <Text style={[styles.buttonText, { color: fg }]}>{props.title}</Text>}
    </Pressable>
  );
}

export function Field(props: TextInputProps & { label: string }) {
  const { colors } = useTheme();
  const { label, style, ...rest } = props;
  return (
    <View style={styles.field}>
      <Text style={[styles.label, { color: colors.muted }]}>{label}</Text>
      <TextInput
        accessibilityLabel={label}
        placeholderTextColor={colors.muted}
        style={[styles.input, { color: colors.text, borderColor: colors.line, backgroundColor: colors.surface }, style]}
        {...rest}
      />
    </View>
  );
}

export function Message(props: { kind?: "error" | "info"; children: ReactNode }) {
  const { colors } = useTheme();
  return <Text accessibilityLiveRegion="polite" style={[styles.msg, { color: props.kind === "error" ? colors.danger : colors.muted }]}>{props.children}</Text>;
}

export function Header(props: { title: string; left?: ReactNode; right?: ReactNode }) {
  const { colors } = useTheme();
  return (
    <View style={[styles.header, { borderBottomColor: colors.line }]}>
      <View style={styles.side}>{props.left}</View>
      <Text accessibilityRole="header" numberOfLines={1} style={[styles.title, { color: colors.text }]}>
        {props.title}
      </Text>
      <View style={[styles.side, { alignItems: "flex-end" }]}>{props.right}</View>
    </View>
  );
}

const styles = StyleSheet.create({
  button: { minHeight: 44, paddingHorizontal: 14, borderRadius: 10, borderWidth: 1, alignItems: "center", justifyContent: "center" },
  buttonText: { fontSize: 15, fontWeight: "600" },
  field: { marginBottom: 12 },
  label: { fontSize: 13, marginBottom: 4 },
  input: { minHeight: 44, borderWidth: 1, borderRadius: 10, paddingHorizontal: 12, fontSize: 16 },
  msg: { fontSize: 14, marginVertical: 6 },
  header: { flexDirection: "row", alignItems: "center", paddingHorizontal: 8, minHeight: 52, borderBottomWidth: StyleSheet.hairlineWidth },
  side: { minWidth: 72 },
  title: { flex: 1, textAlign: "center", fontSize: 17, fontWeight: "700" },
});
