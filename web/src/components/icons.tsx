// Hand-drawn icons for the window chrome, on a 16px grid with a 1.25px
// stroke. The rest of the app uses lucide; these are the few that sit in
// the bars and are seen all the time.
import type { ReactNode } from "react";

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
