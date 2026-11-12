// JavaScript side of the cadenza-fold native module. On devices without the
// module (tests, other platforms) it reports no folds.
import { useEffect, useState } from "react";
import { PixelRatio } from "react-native";
import { requireOptionalNativeModule, type EventSubscription } from "expo";
import { toDp, type Fold, type RawFold } from "../../src/lib/fold";

interface RawLayout {
  folds: RawFold[];
}

interface NativeFold {
  current(): RawLayout;
  addListener(event: "onLayoutChange", listener: (layout: RawLayout) => void): EventSubscription;
}

const native = requireOptionalNativeModule<NativeFold>("CadenzaFold");

/** The current folds in window dp, updated as the device folds and unfolds. */
export function useFolds(): Fold[] {
  const [folds, setFolds] = useState<Fold[]>(() => (native ? toDp(native.current().folds, PixelRatio.get()) : []));
  useEffect(() => {
    if (!native) return;
    const sub = native.addListener("onLayoutChange", (l) => setFolds(toDp(l.folds, PixelRatio.get())));
    setFolds(toDp(native.current().folds, PixelRatio.get()));
    return () => sub.remove();
  }, []);
  return folds;
}
