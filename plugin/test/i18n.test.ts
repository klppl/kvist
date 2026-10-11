import { test, afterEach } from "node:test";
import assert from "node:assert/strict";
import { CATALOGS, Key, language, placeholders, setLanguage, t } from "../src/i18n";
import { en } from "../src/i18n/en";

afterEach(() => setLanguage("en"));

// Every shipped language has every key, with the same {values} as English
// and plural forms its language uses.
for (const [lang, catalog] of Object.entries(CATALOGS)) {
  test(`${lang} is complete`, () => {
    const categories = new Intl.PluralRules(lang).resolvedOptions().pluralCategories;
    for (const key of Object.keys(en) as Key[]) {
      const msg = catalog[key];
      assert.ok(msg !== undefined, `${lang} is missing ${key}`);
      assert.deepEqual(placeholders(msg), placeholders(en[key]), `${lang} ${key}: other {values} than English`);
      assert.equal(typeof msg, typeof en[key], `${lang} ${key}: plural forms where English has none, or the reverse`);
      if (typeof msg !== "string") {
        for (const form of Object.keys(msg)) {
          assert.ok((categories as string[]).includes(form), `${lang} ${key}: ${lang} has no plural form "${form}"`);
        }
      }
    }
    for (const key of Object.keys(catalog)) assert.ok(key in en, `${lang} has ${key}, which English doesn't`);
  });
}

test("t fills values and picks plural forms", () => {
  assert.equal(t("status.uploading", { done: 3, total: 10 }), "uploading 3/10…");
  assert.equal(t("notice.upToDate", { count: 1 }), "kvist: up to date (1 note)");
  assert.equal(t("notice.upToDate", { count: 2 }), "kvist: up to date (2 notes)");
  setLanguage("sv");
  assert.equal(t("notice.upToDate", { count: 1 }), "kvist: redan uppdaterad (1 anteckning)");
  assert.equal(t("notice.upToDate", { count: 0 }), "kvist: redan uppdaterad (0 anteckningar)");
  assert.equal(t("notice.staysPrivate", { name: "Dagbok", reason: t("reason.private_tag") }), "kvist: ”Dagbok” förblir privat: den har den privata taggen.");
});

test("setLanguage falls back to the base language, then English", () => {
  assert.equal(setLanguage("sv-SE"), "sv-SE");
  assert.equal(t("command.publish"), "Publicera nu");
  setLanguage("sv_FI");
  assert.equal(t("command.publish"), "Publicera nu");
  setLanguage("pt-BR");
  assert.equal(t("command.publish"), "Publish now");
  assert.equal(language(), "pt-BR", "plurals and numbers still follow the language asked for");
  setLanguage("not a locale!");
  assert.equal(t("command.publish"), "Publish now");
  assert.equal(language(), "en");
  setLanguage(null);
  assert.equal(language(), "en");
});

test("a word a translation lacks comes from English", () => {
  CATALOGS.xx = { "command.publish": "Xx" };
  try {
    setLanguage("xx");
    assert.equal(t("command.publish"), "Xx");
    assert.equal(t("command.report"), "Open publish report");
  } finally {
    delete CATALOGS.xx;
  }
});
