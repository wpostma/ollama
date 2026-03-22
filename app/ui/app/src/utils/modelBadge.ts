// Color assignments for model family prefixes (first 3 chars, lowercase)
const FIXED_COLORS: Record<string, string> = {
  qwe: "bg-red-600",
  dee: "bg-purple-600",
};

// Pool of colors for unknown prefixes — high contrast with white text
const COLOR_POOL = [
  "bg-teal-600",
  "bg-blue-600",
  "bg-amber-600",
  "bg-emerald-600",
  "bg-rose-600",
  "bg-indigo-600",
  "bg-orange-600",
  "bg-cyan-700",
  "bg-fuchsia-600",
  "bg-lime-700",
  "bg-sky-600",
  "bg-pink-600",
];

// Deterministic hash of a string to an index
function hashToIndex(s: string, max: number): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  }
  return ((h % max) + max) % max;
}

function getColorForPrefix(prefix: string): string {
  if (FIXED_COLORS[prefix]) return FIXED_COLORS[prefix];
  return COLOR_POOL[hashToIndex(prefix, COLOR_POOL.length)];
}

export interface ModelBadgeInfo {
  /** Always exactly 2 characters */
  tag: string;
  bgColor: string;
}

/**
 * Convert a parameter size to a single character:
 *   1-9 → "1"-"9"
 *   10-19 → "A" (tens), 20-29 → "B", ...
 *   Very large (30+b) → X, Y, Z
 */
function sizeToChar(sizeStr: string): string {
  const size = parseFloat(sizeStr);
  if (isNaN(size)) return "?";
  if (size < 10) return Math.round(size).toString();
  if (size < 20) return "A";
  if (size < 30) return "B";
  if (size < 100) return "X";
  if (size < 500) return "Y";
  return "Z";
}

export function parseModelBadge(modelName: string): ModelBadgeInfo {
  const prefix = modelName.slice(0, 3).toLowerCase();
  const letter = modelName[0].toUpperCase();

  // Extract parameter size: look for patterns like "6.7b", "8b", "30b", "120b"
  const sizeMatch = modelName.match(/(\d+(?:\.\d+)?)[bB]/);
  const sizeChar = sizeMatch ? sizeToChar(sizeMatch[1]) : "·";

  const bgColor = getColorForPrefix(prefix);
  return { tag: letter + sizeChar, bgColor };
}
