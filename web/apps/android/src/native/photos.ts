import * as ImagePicker from "expo-image-picker";
import { ImageManipulator, SaveFormat } from "expo-image-manipulator";
import type { UploadItem } from "@cadenza/core";
import { preparePhoto, type PhotoDeps, type PickedPhoto } from "../lib/photo";

const deps: PhotoDeps = {
  async toWebP(uri, width, height, quality) {
    const ref = await ImageManipulator.manipulate(uri).resize({ width, height }).renderAsync();
    const out = await ref.saveAsync({ format: SaveFormat.WEBP, compress: quality });
    return { uri: out.uri, width: out.width, height: out.height };
  },
  async readBlob(uri) {
    const res = await fetch(uri);
    return res.blob();
  },
};

/** Lets the owner pick photos (or take one) and converts them to WebP. Empty when cancelled. */
export async function pickPhotos(source: "library" | "camera"): Promise<UploadItem[]> {
  const opts: ImagePicker.ImagePickerOptions = { mediaTypes: ["images"], quality: 1, allowsMultipleSelection: source === "library", selectionLimit: 5 };
  let res: ImagePicker.ImagePickerResult;
  if (source === "camera") {
    const perm = await ImagePicker.requestCameraPermissionsAsync();
    if (!perm.granted) throw new Error("Camera access is off. Turn it on in Android settings to take a photo.");
    res = await ImagePicker.launchCameraAsync(opts);
  } else {
    res = await ImagePicker.launchImageLibraryAsync(opts);
  }
  if (res.canceled) return [];
  const picked: PickedPhoto[] = res.assets.map((a) => ({ uri: a.uri, width: a.width, height: a.height, fileName: a.fileName ?? null }));
  return Promise.all(picked.map((p, i) => preparePhoto(p, i, deps)));
}
