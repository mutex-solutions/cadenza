import * as SecureStore from "expo-secure-store";
import type { SecureStoreLike } from "../lib/session";

export const secureStore: SecureStoreLike = {
  getItemAsync: (k) => SecureStore.getItemAsync(k),
  setItemAsync: (k, v) => SecureStore.setItemAsync(k, v),
  deleteItemAsync: (k) => SecureStore.deleteItemAsync(k),
};
