// Points represent regional discovery channels, not inferred studio locations.
import type { Market } from "./directory";
export const REGIONS: {
  id: Market;
  label: string;
  lat: number;
  lon: number;
  queries: string[];
}[] = [
  {
    id: "cn",
    label: "中国",
    lat: 35,
    lon: 105,
    queries: ["故事", "科技", "文化", "生活"],
  },
  {
    id: "us",
    label: "美国",
    lat: 39,
    lon: -98,
    queries: ["stories", "technology", "culture", "life"],
  },
  {
    id: "gb",
    label: "英国",
    lat: 54,
    lon: -2,
    queries: ["stories", "technology", "culture", "life"],
  },
  {
    id: "jp",
    label: "日本",
    lat: 36,
    lon: 138,
    queries: ["物語", "テクノロジー", "文化", "生活"],
  },
  {
    id: "de",
    label: "德国",
    lat: 51,
    lon: 10,
    queries: ["Geschichten", "Technologie", "Kultur", "Leben"],
  },
  {
    id: "fr",
    label: "法国",
    lat: 46,
    lon: 2,
    queries: ["histoires", "technologie", "culture", "vie"],
  },
  {
    id: "es",
    label: "西班牙",
    lat: 40,
    lon: -4,
    queries: ["historias", "tecnología", "cultura", "vida"],
  },
  {
    id: "br",
    label: "巴西",
    lat: -14,
    lon: -52,
    queries: ["histórias", "tecnologia", "cultura", "vida"],
  },
  {
    id: "kr",
    label: "韩国",
    lat: 36,
    lon: 128,
    queries: ["이야기", "기술", "문화", "생활"],
  },
  {
    id: "in",
    label: "印度",
    lat: 22,
    lon: 79,
    queries: ["stories", "technology", "culture", "life"],
  },
  {
    id: "au",
    label: "澳大利亚",
    lat: -25,
    lon: 134,
    queries: ["stories", "technology", "culture", "life"],
  },
];
