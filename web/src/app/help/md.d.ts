// A Markdown file imported as its text (Vite's ?raw).
declare module "*.md?raw" {
  const text: string;
  export default text;
}
