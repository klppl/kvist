// Page layouts: the values of a note's layout property. Mirrors
// model.Layouts and the server's reading of the property
// (internal/model/build.go); testdata/parity/layouts.json keeps them in step.

export const LAYOUTS = ["article", "page", "cover", "about", "intro"] as const;
export type Layout = (typeof LAYOUTS)[number];

/**
 * The layout a layout property gives, as the server reads it: "" for an
 * article (also when unset or "default"), the layout's name, or null for
 * a value the server ignores with a warning (an unknown name, a list or
 * a number).
 */
export function readLayout(value: unknown): Exclude<Layout, "article"> | "" | null {
  if (value === undefined || value === null) return "";
  if (typeof value !== "string") return null;
  const l = value.trim().toLowerCase();
  if (l === "" || l === "article" || l === "default") return "";
  return (LAYOUTS as readonly string[]).includes(l) ? (l as Exclude<Layout, "article">) : null;
}
