// Network access is independent of desktop OS access. iOS exposes myco.net;
// desktop Apps that predate it keep working through myco.os.net.
import { myco } from "./myco";
import { t as uiText } from "./i18n";
export type NetworkState = "ready" | "old" | "outside";
export const networkState: NetworkState = !myco
  ? "outside"
  : (myco.net && myco.features?.includes("net")) ||
      (myco.os?.net && myco.features?.includes("os"))
    ? "ready"
    : "old";
export function networkStateText() {
  return networkState === "outside"
    ? uiText("请在 myco App 中打开，才能让应用联网")
    : networkState === "old"
      ? uiText("当前 myco App 尚不支持应用联网，请更新 App 后再试")
      : "";
}
export function network() {
  const provider = myco?.net || myco?.os?.net;
  if (networkState !== "ready" || !provider)
    throw new Error(networkStateText());
  return provider;
}
