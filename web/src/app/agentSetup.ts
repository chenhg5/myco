import { L } from "@/lib/i18n";

/**
 * Sentences for people to hand to their own agent (Claude Code, Codex,
 * claude.ai…): it reads docs/agent-setup.md and sets myco up, or looks
 * into an error, asking before it decides anything for them.
 */
export const SETUP_GUIDE = "https://raw.githubusercontent.com/chenhg5/myco/main/docs/agent-setup.md";
/** The same file through jsDelivr, for networks where GitHub's raw host is slow or blocked. */
export const SETUP_GUIDE_MIRROR = "https://cdn.jsdelivr.net/gh/chenhg5/myco@main/docs/agent-setup.md";

/** Set myco up from nothing: a hub, the App connected, computers and agents. */
export function setupPrompt(): string {
  return L(
    `帮我装好 myco：先读 ${SETUP_GUIDE} （打不开就读 ${SETUP_GUIDE_MIRROR} ），照着做。要我决定的先问我，用 sudo 或改系统设置前也先问我，装完告诉我结果。`,
    `Set up myco for me: read ${SETUP_GUIDE} (if it does not load, ${SETUP_GUIDE_MIRROR}) and follow it. Ask me before deciding anything for me, before using sudo or changing system settings, and tell me the result when you are done.`,
  );
}

/** Look into an error the App showed: its raw words, and the guide's 排错 section. */
export function debugPrompt(detail: string): string {
  const raw = detail.trim().replace(/\s+/g, " ").slice(0, 600);
  return L(
    `myco 出错了：「${raw}」。读 ${SETUP_GUIDE} （打不开就读 ${SETUP_GUIDE_MIRROR} ）里「排错」一节，帮我查原因并修好；用 sudo 或改系统设置前先问我。`,
    `myco shows an error: "${raw}". Read the 排错 (troubleshooting) section of ${SETUP_GUIDE} (if it does not load, ${SETUP_GUIDE_MIRROR}), find the cause and fix it; ask me before using sudo or changing system settings.`,
  );
}
