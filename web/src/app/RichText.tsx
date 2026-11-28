import { Fragment, type ReactNode } from "react";

/** Translate the sentence, retaining code/links as nodes rather than HTML strings. */
export function RichText({ text, values }: { text: string; values: readonly ReactNode[] }) {
  return <>{text.split(/(\{\d+\})/g).map((part, index) => {
    const slot = /^\{(\d+)\}$/.exec(part);
    return <Fragment key={index}>{slot && Number(slot[1]) < values.length ? values[Number(slot[1])] : part}</Fragment>;
  })}</>;
}
