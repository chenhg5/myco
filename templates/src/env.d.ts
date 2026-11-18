/// <reference types="vite/client" />
declare module "marked" {
  export const marked: { parse(src: string): string };
}
