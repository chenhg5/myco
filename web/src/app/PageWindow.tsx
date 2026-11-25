import { useCallback, useEffect, useState } from "react";
import "@/app/app.css";
import { appApi } from "@/appapi";
import { useThemeMode } from "@/lib/theme";
import { PageWindowView, setPageHubs, setPageMe, type PageMeta } from "@/app/pages";

/** One page (an app) in a window of its own, as an installed app opens
 *  (?pagewin=<hub>/<page>): the page alone, with the panel's bridge,
 *  consent and origin, in the App's theme and language. */
export function PageWindow({ hubId, id }: { hubId: string; id: string }) {
  useThemeMode();
  const [ready, setReady] = useState(false);
  useEffect(() => {
    let live = true;
    void (async () => {
      try {
        const [hubs, ov] = await Promise.all([appApi.hubs(), appApi.overview().catch(() => null)]);
        setPageHubs(hubs);
        setPageMe((hid) => ov?.hubs.find((h) => h.id === hid)?.me?.member?.name || "");
      } catch { /* (the page says what it cannot reach) */ }
      if (live) setReady(true);
    })();
    return () => { live = false; };
  }, []);
  const onMeta = useCallback((m: PageMeta) => { if (m.title) document.title = m.title; }, []);
  return (
    <div className="pg-window">
      {ready && <PageWindowView target={{ kind: "page", hubId, id }} onMeta={onMeta} />}
    </div>
  );
}
