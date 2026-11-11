// Hand-drawn icons, on a 16px grid with a 1.25px stroke: the window
// chrome, the sidebar's pages, and what a node or a machine is. Everything
// else (actions inside pages and dialogs) uses lucide.
import type { ComponentType, ReactNode } from "react";

/** What a place that takes "an icon" accepts: one of these or a lucide icon. */
export type IconType = ComponentType<{ className?: string }>;

function Icon({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 16 16"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.25"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
    >
      {children}
    </svg>
  );
}

type Props = { className?: string };

/** Show or hide the sidebar: a window with its left pane marked. */
export function IconSidebar({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="1.6" y="2.6" width="12.8" height="10.8" rx="2.6" />
      <rect x="3.9" y="4.9" width="1.9" height="6.2" rx="0.95" fill="currentColor" stroke="none" />
    </Icon>
  );
}

/** Start a conversation: a speech bubble with a plus. */
export function IconNewChat({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M2.6 13.4V5.2a2.6 2.6 0 0 1 2.6-2.6h5.6a2.6 2.6 0 0 1 2.6 2.6v3.6a2.6 2.6 0 0 1-2.6 2.6H5.3L2.6 13.4Z" />
      <path d="M8 5v4M6 7h4" />
    </Icon>
  );
}

/** Tuck a column away to the left. */
export function IconCollapseLeft({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M7.4 4.6 4 8l3.4 3.4M12 4.6 8.6 8l3.4 3.4" />
    </Icon>
  );
}

/** Bring a tucked-away column back. */
export function IconExpandRight({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M8.6 4.6 12 8l-3.4 3.4M4 4.6 7.4 8 4 11.4" />
    </Icon>
  );
}

const BUBBLE = "M2.6 13.4V5.2a2.6 2.6 0 0 1 2.6-2.6h5.6a2.6 2.6 0 0 1 2.6 2.6v3.6a2.6 2.6 0 0 1-2.6 2.6H5.3L2.6 13.4Z";

/* ---------- sidebar pages ---------- */

/** Nodes: three of them, linked. */
export function IconNodes({ className }: Props) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="3.7" r="1.75" />
      <circle cx="3.7" cy="11.9" r="1.75" />
      <circle cx="12.3" cy="11.9" r="1.75" />
      <path d="M7.19 5.25 4.51 10.35M8.81 5.25l2.68 5.1M5.45 11.9h5.1" />
    </Icon>
  );
}

/** Machines: a display on its stand. */
export function IconMachine({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="1.9" y="2.6" width="12.2" height="8.4" rx="1.9" />
      <path d="M8 11v2.4M5.4 13.4h5.2" />
    </Icon>
  );
}

/** Conversations, and a conversation with you in it. */
export function IconChat({ className }: Props) {
  return (
    <Icon className={className}>
      <path d={BUBBLE} />
    </Icon>
  );
}

/** A project: a folder. */
export function IconProject({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M2.1 4.6a1.5 1.5 0 0 1 1.5-1.5h2.6l1.6 1.7h4.6a1.5 1.5 0 0 1 1.5 1.5v5.2a1.5 1.5 0 0 1-1.5 1.5H3.6a1.5 1.5 0 0 1-1.5-1.5Z" />
    </Icon>
  );
}

/* ---------- what a node is ---------- */

/** Claude Code: an asterisk. */
export function IconAsterisk({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M8 2.5v11M3.24 5.25l9.52 5.5M12.76 5.25l-9.52 5.5" />
    </Icon>
  );
}

/** Codex: code brackets. */
export function IconCode({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M5.4 4.9 2.4 8l3 3.1M10.6 4.9l3 3.1-3 3.1M9.1 3.5 6.9 12.5" />
    </Icon>
  );
}

/** Gemini CLI: a four-point star. */
export function IconStar({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M8 2.2C8.4 5.4 10.6 7.6 13.8 8 10.6 8.4 8.4 10.6 8 13.8 7.6 10.6 5.4 8.4 2.2 8 5.4 7.6 7.6 5.4 8 2.2Z" />
    </Icon>
  );
}

/** Cursor: a pointer. */
export function IconPointer({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M3.3 2.9l9.6 3.8-4 1.6-1.6 4L3.3 2.9Z" />
    </Icon>
  );
}

/** Any other agent CLI: a small robot. */
export function IconBot({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="2.6" y="5.6" width="10.8" height="7.6" rx="2.4" />
      <path d="M8 5.6V3.3M6.1 8.6v1.6M9.9 8.6v1.6" />
      <circle cx="8" cy="2.7" r="0.7" fill="currentColor" stroke="none" />
    </Icon>
  );
}

/** A shell node: the prompt. */
export function IconPrompt({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M2.9 4.4 7.2 8l-4.3 3.6M8.6 12h4.6" />
    </Icon>
  );
}

/** An exec node, or a CLI that lives in a terminal: the prompt in a window. */
export function IconTerminal({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="2" y="2.6" width="12" height="10.8" rx="2.4" />
      <path d="M4.9 6.2 7 8l-2.1 1.8M8.5 10.1h2.6" />
    </Icon>
  );
}

/** A person. */
export function IconPerson({ className }: Props) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="5.3" r="2.6" />
      <path d="M3 13.5a5 5 0 0 1 10 0" />
    </Icon>
  );
}

/** A machine as a node: a server, two units. */
export function IconServer({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="2.2" y="2.5" width="11.6" height="4.7" rx="1.6" />
      <rect x="2.2" y="8.8" width="11.6" height="4.7" rx="1.6" />
      <circle cx="4.9" cy="4.85" r="0.6" fill="currentColor" stroke="none" />
      <circle cx="4.9" cy="11.15" r="0.6" fill="currentColor" stroke="none" />
    </Icon>
  );
}

/** A laptop (macOS machines). */
export function IconLaptop({ className }: Props) {
  return (
    <Icon className={className}>
      <rect x="3" y="3" width="10" height="7.2" rx="1.6" />
      <path d="M1.5 12.9h13" />
    </Icon>
  );
}

/** A cloud (sessions with no machine). */
export function IconCloud({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M4.6 12.6a2.9 2.9 0 0 1-.5-5.76 4 4 0 0 1 7.74-.5A3.15 3.15 0 0 1 11.4 12.6Z" />
    </Icon>
  );
}

/** A conversation between two nodes. */
export function IconExchange({ className }: Props) {
  return (
    <Icon className={className}>
      <path d="M3 5.5h9.8M10.4 3.1l2.4 2.4-2.4 2.4M13 10.5H3.2M5.6 8.1 3.2 10.5l2.4 2.4" />
    </Icon>
  );
}
