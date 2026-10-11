// The plugin's words, in the language Obsidian is set to. Pure (no
// Obsidian import), so the publisher, scanner and tests can use it too.
//
// Each language is a file in this folder: a catalog keyed like en.ts, the
// source of every key. A word a language doesn't have comes from English,
// so a partial translation works. Words name their values, {count} or
// {name}, so a translation can put them in any order, and a word with
// plural forms is a table of Intl.PluralRules categories (one, few, many,
// other…), picked by {count}.
//
// To add a language: copy sv.ts, translate it, and list it in CATALOGS.

import { en } from "./en";
import { sv } from "./sv";

/** A word, or its plural forms by Intl.PluralRules category. */
export type Message = string | ({ other: string } & Partial<Record<Intl.LDMLPluralRule, string>>);
export type Key = keyof typeof en;
/** A translation: any of en's keys, and only those. */
export type Catalog = { [K in Key]?: Message };
export type Vars = Record<string, string | number>;

/** The languages the plugin speaks, by Obsidian's language codes. */
export const CATALOGS: Record<string, Catalog> = { en, sv };

let lang = "en";
let chain: Catalog[] = [en];

/**
 * Sets the language from a code such as "sv", "pt-BR" or "zh-TW": that
 * language, then its base language, then English. Returns the language
 * used for plurals and numbers.
 */
export function setLanguage(code: string | null | undefined): string {
  const want = (code ?? "").trim().replace(/_/g, "-");
  const lower = want.toLowerCase();
  const base = lower.split("-")[0];
  const find = (c: string) => Object.keys(CATALOGS).find((k) => k.toLowerCase() === c);
  const exact = find(lower);
  const parent = base !== lower ? find(base) : undefined;
  chain = [exact, parent, "en"].filter((c, i, a): c is string => !!c && a.indexOf(c) === i).map((c) => CATALOGS[c]);
  lang = exact ?? parent ?? "en";
  try {
    // Plurals and numbers follow the language asked for, even untranslated.
    if (want) lang = Intl.getCanonicalLocales(want)[0] ?? lang;
  } catch {
    // not a valid locale tag: keep the catalog's
  }
  return lang;
}

/** The language t uses, for dates and numbers: toLocaleTimeString(language()). */
export function language(): string {
  return lang;
}

/** Returns the word for key, with vars filled in. */
export function t(key: Key, vars?: Vars): string {
  let msg: Message | undefined;
  for (const c of chain) {
    msg = c[key];
    if (msg !== undefined) break;
  }
  if (msg === undefined) return key;
  let s: string;
  if (typeof msg === "string") {
    s = msg;
  } else {
    const n = Number(vars?.count ?? 0);
    s = msg[plural(n)] ?? msg.other;
  }
  return vars ? fill(s, vars) : s;
}

function plural(n: number): Intl.LDMLPluralRule {
  try {
    return new Intl.PluralRules(lang).select(n);
  } catch {
    return n === 1 ? "one" : "other";
  }
}

function fill(s: string, vars: Vars): string {
  return s.replace(/\{(\w+)\}/g, (m, name: string) => {
    const v = vars[name];
    if (v === undefined) return m;
    return typeof v === "number" ? v.toLocaleString(lang) : v;
  });
}

/** The value names a word uses, such as ["count", "name"]; for tests. */
export function placeholders(msg: Message): string[] {
  const forms = typeof msg === "string" ? [msg] : Object.values(msg).filter((s): s is string => typeof s === "string");
  const names = new Set<string>();
  for (const s of forms) for (const m of s.matchAll(/\{(\w+)\}/g)) names.add(m[1]);
  return [...names].sort();
}
