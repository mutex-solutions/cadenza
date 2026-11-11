import { useColorScheme } from "react-native";

export interface Colors {
  bg: string;
  surface: string;
  line: string;
  text: string;
  muted: string;
  accent: string;
  accentText: string;
  danger: string;
  good: string;
}

const light: Colors = { bg: "#ffffff", surface: "#f3f4f6", line: "#e5e7eb", text: "#111827", muted: "#4b5563", accent: "#0369a1", accentText: "#ffffff", danger: "#b91c1c", good: "#047857" };
const dark: Colors = { bg: "#0b0f14", surface: "#151c25", line: "#233143", text: "#e8eef6", muted: "#a2b0c1", accent: "#7dd3fc", accentText: "#0b0f14", danger: "#f59393", good: "#62d9ac" };

export function useTheme(): { colors: Colors; scheme: "light" | "dark" } {
  const scheme = useColorScheme() === "dark" ? "dark" : "light";
  return { colors: scheme === "dark" ? dark : light, scheme };
}
