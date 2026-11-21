// The sandbox never contacts external hosts directly. Catalogs, images and
// HLS requests use the App network bridge, with bounded response sizes.
import { fromBase64 } from "../_lib/os";
import { network } from "../_lib/net";
import { t as uiText } from "../_lib/i18n";
import { httpURL, parseCatalog, requestURL, type Source } from "./catalog";
export async function request(
  url: string,
  binary = false,
  headers: Record<string, string> = {},
  limit = 16 * 1024 * 1024,
) {
  const address = httpURL(url);
  if (!address) throw new Error(uiText("这个播放地址不受支持"));
  const result = await network().fetch({
    url: address,
    headers,
    response: binary ? "base64" : "text",
    timeout_ms: 20000,
    max_bytes: limit,
  });
  if (result.status < 200 || result.status >= 300)
    throw new Error(
      uiText("连接失败（HTTP {0}），请稍后重试或换个影视源", result.status),
    );
  if (result.truncated)
    throw new Error(uiText("返回的内容太大，请换一条播放线路"));
  return result;
}
export async function catalog(
  id: string,
  source: Source,
  params: Parameters<typeof requestURL>[1] = {},
) {
  const response = await request(
    requestURL(source, params),
    false,
    {},
    4 * 1024 * 1024,
  );
  let raw: unknown;
  try {
    raw = JSON.parse(response.body);
  } catch {
    throw new Error(
      uiText("这个地址没有返回兼容的影视列表，请核对 MacCMS JSON 接口"),
    );
  }
  return parseCatalog(raw, id, source.endpoint);
}
export async function posterURL(url: string) {
  const r = await request(url, true, {}, 2 * 1024 * 1024);
  const type = (r.headers["content-type"] || "").split(";")[0].toLowerCase();
  if (
    ![
      "image/jpeg",
      "image/png",
      "image/webp",
      "image/gif",
      "image/avif",
    ].includes(type)
  )
    throw new Error("image");
  return URL.createObjectURL(new Blob([fromBase64(r.body)], { type }));
}
