// The kvist Obsidian plugin: decides what is public, pushes it to a kvist
// server, and reports what stays private. Uses only the Obsidian API, so it
// works on desktop and mobile.

import {
  App, ItemView, Menu, Modal, Notice, Platform, Plugin, PluginSettingTab, Setting, TAbstractFile, TFile,
  WorkspaceLeaf, getAllTags, normalizePath, requestUrl,
} from "obsidian";
import { Client, Transport } from "./client";
import { HashCache, HashEntry } from "./hash";
import { ProtocolError, Regression, Rules, SETTINGS_NOTE_NAME, SITE_NOTE_PATH, Warning } from "./protocol";
import { DeviceState, PublishResult, Publisher, StaleCancelled } from "./publisher";
import { evaluate, isNote, isSettingsNote } from "./rules";
import { LeakItem, NoteMeta, VaultFile, VaultLike, findSettingsNote } from "./scan";
import { noteURL } from "./slug";

const VIEW_REPORT = "kvist-report";

/** The settings note a new site starts with. Empty properties are ignored. */
const SETTINGS_TEMPLATE = `---
title:
description:
author:
language:
home:
groups: []
accent:
footer:
image:
favicon:
---
Settings for your kvist website. This note is never published as a page. Fill in the properties above, then publish. Empty ones keep their default.

- **title** and **description**: the site's name and tagline.
- **language**: a code such as \`en\` or \`sv\`, for the site's own words and dates.
- **home**: a note to use as the home page, written as a link: \`[[Welcome]]\`. Empty: a generated home page.
- **groups**: tags that group the menu, such as \`articles\` and \`projects\`. Empty: your folders.
- **accent**: the color of links, such as \`#3f7d4e\`.
- **footer**: text at the bottom of the menu. Markdown works.
- **image**: the picture shown when a page is shared, as a link: \`[[preview.png]]\`.
- **favicon**: the icon in browser tabs, as a link: \`[[icon.png]]\`.

More settings, such as a profile with your picture and links, or visitor statistics, are in the documentation: https://klppl.github.io/kvist/customizing.html

## Links

Each list item that is a single link becomes a menu link, in order. Links can point to websites, to addresses like /about/, or to published notes with \`[[wikilinks]]\`. For example:

%%
- [About](/about/)
- [Mastodon](https://mastodon.social/@you)
%%
`;

/** Shared settings (data.json; may sync between devices). */
interface SharedSettings {
  serverURL: string;
  site: string;
}

/** Device-only settings (Obsidian local storage; never synced). */
interface LocalSettings {
  token: string;
  deviceName: string;
  autoPublish: boolean;
  debounceSeconds: number;
  state: DeviceState;
  hashes: Record<string, HashEntry>;
}

const DEFAULT_SHARED: SharedSettings = { serverURL: "", site: "" };

type Status = "idle" | "pending" | "publishing" | "ok" | "warnings" | "stale" | "error";

export default class KvistPlugin extends Plugin {
  shared: SharedSettings = { ...DEFAULT_SHARED };
  local!: LocalSettings;
  cache!: HashCache;
  statusEl!: HTMLElement;
  status: Status = "idle";
  statusText = "";
  lastResult?: PublishResult;
  lastWarnings: Warning[] = [];
  lastLeaks: LeakItem[] = [];
  rules?: Rules;
  private timer?: number;
  private running = false;
  private again = false;

  async onload() {
    this.shared = Object.assign({}, DEFAULT_SHARED, await this.loadData());
    this.local = this.loadLocal();
    this.cache = new HashCache(this.local.hashes);

    this.statusEl = this.addStatusBarItem();
    this.statusEl.addClass("mod-clickable", "kvist-status");
    this.statusEl.onClickEvent(() => this.openReport());
    this.setStatus("idle", "kvist");

    this.registerView(VIEW_REPORT, (leaf) => new ReportView(leaf, this));
    this.addSettingTab(new KvistSettingTab(this.app, this));

    this.addCommand({ id: "publish-now", name: "Publish now", callback: () => this.publish(false) });
    this.addCommand({
      id: "toggle-publish",
      name: "Toggle publish for current note",
      checkCallback: (checking) => {
        const f = this.app.workspace.getActiveFile();
        if (!f || f.extension !== "md") return false;
        if (!checking) void this.togglePublish(f);
        return true;
      },
    });
    this.addCommand({ id: "open-report", name: "Open publish report", callback: () => this.openReport() });
    this.addCommand({ id: "settings-note", name: "Open site settings note", callback: () => this.openSettingsNote() });
    this.addCommand({
      id: "open-published",
      name: "Open published page",
      checkCallback: (checking) => {
        const f = this.app.workspace.getActiveFile();
        if (!f || f.extension !== "md") return false;
        if (!checking) this.openPublished(f);
        return true;
      },
    });

    this.registerEvent(
      this.app.workspace.on("file-menu", (menu: Menu, file: TAbstractFile) => {
        if (!(file instanceof TFile) || file.extension !== "md" || isSettingsNote(file.path)) return;
        const published = this.decide(file)?.published;
        menu.addItem((item) =>
          item.setTitle(published ? "Unpublish (kvist)" : "Publish (kvist)").setIcon("globe").onClick(() => this.togglePublish(file)),
        );
      }),
    );

    const changed = () => this.schedule();
    this.app.workspace.onLayoutReady(() => {
      this.registerEvent(this.app.vault.on("create", changed));
      this.registerEvent(this.app.vault.on("modify", changed));
      this.registerEvent(this.app.vault.on("delete", changed));
      this.registerEvent(this.app.vault.on("rename", changed));
      this.registerEvent(this.app.metadataCache.on("changed", changed));
    });
  }

  onunload() {
    if (this.timer) window.clearTimeout(this.timer);
  }

  // ---- settings ----

  private loadLocal(): LocalSettings {
    const saved = (this.app.loadLocalStorage("kvist-local") ?? {}) as Partial<LocalSettings>;
    return {
      token: saved.token ?? "",
      deviceName: saved.deviceName ?? (Platform.isMobile ? "Mobile" : "Desktop"),
      autoPublish: saved.autoPublish ?? false,
      debounceSeconds: saved.debounceSeconds ?? 30,
      state: saved.state ?? { clientId: randomId(), baseRevision: "" },
      hashes: saved.hashes ?? {},
    };
  }

  saveLocal() {
    this.local.hashes = this.cache.toJSON();
    this.app.saveLocalStorage("kvist-local", this.local);
  }

  async saveShared() {
    await this.saveData(this.shared);
  }

  /** Forgets the server, site, token and this device's publish state. */
  async resetSettings() {
    if (this.timer) window.clearTimeout(this.timer);
    this.timer = undefined;
    this.shared = { ...DEFAULT_SHARED };
    await this.saveShared();
    this.app.saveLocalStorage("kvist-local", null);
    this.local = this.loadLocal();
    this.cache = new HashCache(this.local.hashes);
    this.saveLocal();
    this.rules = undefined;
    this.lastResult = undefined;
    this.lastWarnings = [];
    this.lastLeaks = [];
    this.setStatus("idle", "kvist");
  }

  publishing(): boolean {
    return this.running;
  }

  configured(): boolean {
    return !!(this.shared.serverURL && this.shared.site && this.local.token);
  }

  // ---- status ----

  setStatus(s: Status, text: string) {
    this.status = s;
    this.statusText = text;
    const icon = { idle: "○", pending: "◔", publishing: "◑", ok: "●", warnings: "▲", stale: "◆", error: "✕" }[s];
    this.statusEl.setText(`${icon} ${text}`);
    this.statusEl.setAttribute("aria-label", "kvist: " + text + " (click for the report)");
    this.statusEl.toggleClass("kvist-warn", s === "warnings" || s === "stale");
    this.statusEl.toggleClass("kvist-error", s === "error");
    this.app.workspace.getLeavesOfType(VIEW_REPORT).forEach((l) => (l.view as ReportView).render());
  }

  // ---- publishing ----

  schedule() {
    if (!this.local.autoPublish || !this.configured() || this.status === "stale") return;
    if (this.timer) window.clearTimeout(this.timer);
    if (!this.running) this.setStatus("pending", "changes pending");
    this.timer = window.setTimeout(() => void this.publish(true), Math.max(5, this.local.debounceSeconds) * 1000);
  }

  async publish(auto: boolean) {
    if (!this.configured()) {
      new Notice("kvist: set the server URL, site and token in the settings first.");
      return;
    }
    if (this.running) {
      this.again = true;
      return;
    }
    this.running = true;
    try {
      const client = new Client(this.shared.serverURL, this.shared.site, this.local.token, obsidianTransport);
      const pub = new Publisher(
        client, new ObsidianVault(this.app), this.cache, this.local.state,
        (s) => { this.local.state = s; this.saveLocal(); },
        this.local.deviceName, Platform.isMobile ? "mobile" : "desktop", this.manifest.version,
      );
      this.setStatus("publishing", "publishing…");
      const res = await pub.publish({
        skipIfUnchanged: auto,
        waitForBuild: true,
        onProgress: (m) => this.setStatus("publishing", m.toLowerCase()),
        confirmStale: auto ? undefined : (msg, regs) => new StaleModal(this.app, msg, regs).ask(),
      });
      this.saveLocal();
      this.lastResult = res;
      this.rules = res.site.rules;
      this.lastLeaks = res.scan.leaks;
      if (!res.skipped) this.lastWarnings = [...res.warnings, ...(res.build?.warnings ?? [])];
      const time = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
      if (res.build?.state === "failed") {
        this.setStatus("error", "build failed");
        new Notice(`kvist: the site build failed: ${res.build.error ?? ""}`);
      } else if (this.lastWarnings.length) {
        this.setStatus("warnings", `published ${time} · ${this.lastWarnings.length} warning${this.lastWarnings.length === 1 ? "" : "s"}`);
      } else {
        this.setStatus("ok", `published ${time}`);
      }
      if (!auto) {
        const n = res.scan.files.filter((f) => isNote(f.path)).length;
        new Notice(res.unchanged ? `kvist: up to date (${n} notes)` : `kvist: published ${n} notes (${res.uploaded} files uploaded)`);
      }
    } catch (e) {
      if (e instanceof StaleCancelled || (e instanceof ProtocolError && e.code === "stale_state")) {
        this.setStatus("stale", "newer content on the server");
        new Notice("kvist: the server has newer content than this device. Sync your vault, then use “Publish now” to review.");
      } else {
        const msg = e instanceof Error ? e.message : String(e);
        this.setStatus("error", "publish failed");
        new Notice("kvist: " + msg);
        console.error("kvist", e);
      }
    } finally {
      this.running = false;
      if (this.again) {
        this.again = false;
        this.schedule();
      }
    }
  }

  // ---- single notes ----

  decide(file: TFile) {
    if (!this.rules) return undefined;
    const cache = this.app.metadataCache.getFileCache(file);
    return evaluate(this.rules, file.path, { frontmatter: cache?.frontmatter, tags: cache ? getAllTags(cache) ?? [] : [] });
  }

  async ensureRules(): Promise<Rules | undefined> {
    if (this.rules || !this.configured()) return this.rules;
    try {
      const client = new Client(this.shared.serverURL, this.shared.site, this.local.token, obsidianTransport);
      this.rules = (await client.siteInfo()).rules;
    } catch (e) {
      new Notice("kvist: could not reach the server: " + (e instanceof Error ? e.message : e));
    }
    return this.rules;
  }

  async togglePublish(file: TFile) {
    if (isSettingsNote(file.path)) {
      new Notice("kvist: this is the site's settings note. It is never published as a page.");
      return;
    }
    const rules = await this.ensureRules();
    if (!rules) return;
    const before = this.decide(file)!;
    const key = rules.frontmatter_key || "publish";
    await this.app.fileManager.processFrontMatter(file, (fm) => {
      for (const k of Object.keys(fm)) if (k.toLowerCase() === key.toLowerCase() && k !== key) delete fm[k];
      fm[key] = !before.published;
    });
    // processFrontMatter writes the file; read the decision from the new frontmatter.
    const cache = this.app.metadataCache.getFileCache(file);
    const after = evaluate(rules, file.path, {
      frontmatter: { ...(cache?.frontmatter ?? {}), [key]: !before.published },
      tags: cache ? getAllTags(cache) ?? [] : [],
    });
    if (after.published === before.published) {
      const why = after.reason === "excluded_folder" ? "it is in an excluded folder" : after.reason === "private_tag" ? "it has the private tag" : after.reason;
      new Notice(`kvist: “${file.basename}” stays ${after.published ? "public" : "private"}: ${why}.`);
    } else {
      new Notice(`kvist: “${file.basename}” will be ${after.published ? "published" : "unpublished"} on the next publish.`);
    }
  }

  /** Opens the settings note (_site.md), creating it from a template if there is none. */
  async openSettingsNote() {
    let path: string | undefined;
    try {
      path = findSettingsNote(this.app.vault.getMarkdownFiles().map((f) => f.path));
    } catch (e) {
      new Notice("kvist: " + (e instanceof Error ? e.message : e));
      return;
    }
    if (!path) {
      // Next to the public notes if the rules name a folder, else at the top.
      const rules = await this.ensureRules();
      const folder = (rules?.always_public_folders ?? []).map((f) => f.replace(/^\/+|\/+$/g, "")).find((f) => f !== "") ?? "";
      if (folder && !this.app.vault.getAbstractFileByPath(folder)) await this.app.vault.createFolder(folder);
      path = normalizePath(folder ? `${folder}/${SETTINGS_NOTE_NAME}` : SETTINGS_NOTE_NAME);
      await this.app.vault.create(path, SETTINGS_TEMPLATE);
      new Notice("kvist: created the site settings note. Fill it in, then publish.");
    }
    const file = this.app.vault.getAbstractFileByPath(path);
    if (file instanceof TFile) await this.app.workspace.getLeaf(false).openFile(file);
  }

  openPublished(file: TFile) {
    const base = this.lastResult?.site.base_url;
    if (!base) {
      new Notice("kvist: publish once first, so the plugin knows the site address.");
      return;
    }
    const d = this.decide(file);
    if (!d?.published) {
      new Notice("kvist: this note is not published.");
      return;
    }
    const fm = this.app.metadataCache.getFileCache(file)?.frontmatter;
    window.open(base + noteURL(file.path, fm?.permalink, this.lastResult?.site.root_folder));
  }

  async openReport() {
    const existing = this.app.workspace.getLeavesOfType(VIEW_REPORT)[0];
    const leaf = existing ?? this.app.workspace.getRightLeaf(false);
    if (!leaf) return;
    await leaf.setViewState({ type: VIEW_REPORT, active: true });
    this.app.workspace.revealLeaf(leaf);
  }
}

function randomId(): string {
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}

const obsidianTransport: Transport = async (req) => {
  const res = await requestUrl({ url: req.url, method: req.method, headers: req.headers, body: req.body, contentType: req.contentType, throw: false });
  return { status: res.status, text: res.text };
};

/** Adapts Obsidian's vault and metadata cache to VaultLike. */
class ObsidianVault implements VaultLike {
  constructor(private app: App) {}

  files(): VaultFile[] {
    return this.app.vault.getFiles().map((f) => ({ path: f.path, mtime: f.stat.mtime, size: f.stat.size }));
  }

  async read(path: string): Promise<ArrayBuffer> {
    const f = this.app.vault.getAbstractFileByPath(path);
    if (!(f instanceof TFile)) throw new Error(`${path} disappeared`);
    return this.app.vault.readBinary(f);
  }

  meta(path: string): NoteMeta | null {
    const f = this.app.vault.getAbstractFileByPath(path);
    if (!(f instanceof TFile)) return null;
    const c = this.app.metadataCache.getFileCache(f);
    if (!c) return null;
    return {
      frontmatter: c.frontmatter,
      tags: getAllTags(c) ?? [],
      links: [
        ...(c.links ?? []).map((l) => ({ link: l.link, original: l.original, embed: false })),
        ...(c.embeds ?? []).map((l) => ({ link: l.link, original: l.original, embed: true })),
      ],
    };
  }

  resolve(linkpath: string, source: string): string | null {
    return this.app.metadataCache.getFirstLinkpathDest(linkpath, source)?.path ?? null;
  }
}

class StaleModal extends Modal {
  private resolve?: (ok: boolean) => void;
  constructor(app: App, private message: string, private regressions: Regression[]) {
    super(app);
  }
  ask(): Promise<boolean> {
    return new Promise((r) => {
      this.resolve = r;
      this.open();
    });
  }
  onOpen() {
    const { contentEl } = this;
    contentEl.createEl("h2", { text: "The server has newer content" });
    contentEl.createEl("p", { text: this.message });
    contentEl.createEl("p", {
      text: "This usually means another device published changes this device hasn't synced yet. Publishing now would restore these older versions:",
    });
    const ul = contentEl.createEl("ul");
    for (const r of this.regressions.slice(0, 30)) {
      ul.createEl("li", { text: `${r.path} — ${r.kind === "older" ? "older version here" : "missing here, changed on the server"}` });
    }
    if (this.regressions.length > 30) ul.createEl("li", { text: `…and ${this.regressions.length - 30} more` });
    new Setting(contentEl)
      .addButton((b) => b.setButtonText("Cancel").onClick(() => this.done(false)))
      .addButton((b) => b.setButtonText("Publish anyway").setWarning().onClick(() => this.done(true)));
  }
  private done(ok: boolean) {
    this.resolve?.(ok);
    this.resolve = undefined;
    this.close();
  }
  onClose() {
    this.resolve?.(false);
    this.contentEl.empty();
  }
}

class ReportView extends ItemView {
  constructor(leaf: WorkspaceLeaf, private plugin: KvistPlugin) {
    super(leaf);
  }
  getViewType() {
    return VIEW_REPORT;
  }
  getDisplayText() {
    return "kvist publish report";
  }
  getIcon() {
    return "globe";
  }
  async onOpen() {
    this.render();
  }

  render() {
    const el = this.contentEl;
    el.empty();
    el.addClass("kvist-report");
    el.createEl("h4", { text: "kvist" });
    el.createEl("p", { text: this.plugin.statusText || "Not published yet.", cls: "kvist-report-status" });
    new Setting(el).addButton((b) => b.setButtonText("Publish now").setCta().onClick(() => this.plugin.publish(false)));

    const res = this.plugin.lastResult;
    if (!res) {
      el.createEl("p", { text: "Publish once to see what is public and what stays private.", cls: "setting-item-description" });
      return;
    }
    const notes = [...res.scan.decisions.entries()].filter(([, d]) => d.published).length;
    el.createEl("p", { text: `${notes} notes and ${res.scan.files.length - notes} other files are public.` });

    this.section(el, "Links to notes that are not published", "These links show as plain text on the site.",
      this.plugin.lastLeaks.filter((l) => l.kind === "unpublished_link"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, "Embeds of notes that are not published", "These embeds are left out of the page.",
      this.plugin.lastLeaks.filter((l) => l.kind === "unpublished_embed"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, "Attachments in excluded folders", "Published notes reference these, but they stay private.",
      this.plugin.lastLeaks.filter((l) => l.kind === "excluded_attachment"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, "From the server", "Warnings from the last publish and build.",
      this.plugin.lastWarnings, (w) => [w.path === SITE_NOTE_PATH ? res.scan.settingsNote ?? "" : w.path ?? "", w.message]);
  }

  private section<T>(el: HTMLElement, title: string, desc: string, items: T[], row: (t: T) => [string, string]) {
    if (!items.length) return;
    el.createEl("h5", { text: `${title} (${items.length})` });
    el.createEl("p", { text: desc, cls: "setting-item-description" });
    const ul = el.createEl("ul", { cls: "kvist-report-list" });
    for (const it of items.slice(0, 200)) {
      const [path, text] = row(it);
      const li = ul.createEl("li");
      if (path) {
        const a = li.createEl("a", { text: path, href: "#" });
        a.onClickEvent((e) => {
          e.preventDefault();
          void this.app.workspace.openLinkText(path, "", false);
        });
        li.createSpan({ text: " " });
      }
      li.createSpan({ text, cls: "kvist-muted" });
    }
  }
}

class KvistSettingTab extends PluginSettingTab {
  constructor(app: App, private plugin: KvistPlugin) {
    super(app, plugin);
  }

  display() {
    const { containerEl } = this;
    const p = this.plugin;
    containerEl.empty();
    containerEl.createEl("p", {
      text: "The server decides the publish rules. Create a token on the server with: kvist token create --site <site> --name <device>.",
      cls: "setting-item-description",
    });
    new Setting(containerEl).setName("Server URL").setDesc("For example https://kvist.example.com")
      .addText((t) => t.setPlaceholder("https://").setValue(p.shared.serverURL).onChange(async (v) => {
        p.shared.serverURL = v.trim();
        p.rules = undefined;
        await p.saveShared();
      }));
    new Setting(containerEl).setName("Site").setDesc("The site id from the server config")
      .addText((t) => t.setValue(p.shared.site).onChange(async (v) => {
        p.shared.site = v.trim();
        p.rules = undefined;
        await p.saveShared();
      }));

    new Setting(containerEl).setName("Site settings")
      .setDesc(`Title, home page, menu and colors live in a note called ${SETTINGS_NOTE_NAME}, so you can change them from any device.`)
      .addButton((b) => b.setButtonText("Open settings note").onClick(() => p.openSettingsNote()));

    containerEl.createEl("h3", { text: "This device" });
    containerEl.createEl("p", {
      text: "These settings are stored on this device only and are not synced to your other devices.",
      cls: "setting-item-description",
    });
    new Setting(containerEl).setName("Token").setDesc("Use a separate token per device, so you can revoke one.")
      .addText((t) => {
        t.inputEl.type = "password";
        t.setValue(p.local.token).onChange((v) => {
          p.local.token = v.trim();
          p.saveLocal();
        });
      });
    new Setting(containerEl).setName("Device name").setDesc("Shown in warnings and on the server.")
      .addText((t) => t.setValue(p.local.deviceName).onChange((v) => {
        p.local.deviceName = v.trim();
        p.saveLocal();
      }));
    new Setting(containerEl).setName("Publish automatically")
      .setDesc("Publish after changes settle. Turn this on for one main device only: a device that hasn't finished syncing could otherwise publish older notes.")
      .addToggle((t) => t.setValue(p.local.autoPublish).onChange((v) => {
        p.local.autoPublish = v;
        p.saveLocal();
      }));
    new Setting(containerEl).setName("Wait before publishing (seconds)")
      .addText((t) => t.setValue(String(p.local.debounceSeconds)).onChange((v) => {
        const n = parseInt(v, 10);
        if (n >= 5) {
          p.local.debounceSeconds = n;
          p.saveLocal();
        }
      }));
    new Setting(containerEl).setName("Test connection").addButton((b) => b.setButtonText("Test").onClick(async () => {
      try {
        const c = new Client(p.shared.serverURL, p.shared.site, p.local.token, obsidianTransport);
        const info = await c.siteInfo();
        p.rules = info.rules;
        new Notice(`kvist: connected to “${info.site}” (${info.base_url}).`);
      } catch (e) {
        new Notice("kvist: " + (e instanceof Error ? e.message : e));
      }
    }));

    containerEl.createEl("h3", { text: "Reset" });
    new Setting(containerEl).setName("Reset settings")
      .setDesc("Forget the server, site, token and this device's settings, for example to connect to a new site. Your notes are not touched.")
      .addButton((b) => b.setButtonText("Reset").setWarning().onClick(async () => {
        if (p.publishing()) {
          new Notice("kvist: wait for the publish to finish.");
          return;
        }
        const ok = await new ConfirmModal(this.app, "Reset kvist settings?",
          "This clears the server URL, site and token, and this device's publish state. On other devices, the server URL and site are cleared once this vault's settings sync to them.",
          "Reset").ask();
        if (!ok) return;
        await p.resetSettings();
        new Notice("kvist: settings reset.");
        this.display();
      }));
  }
}

class ConfirmModal extends Modal {
  private resolve?: (ok: boolean) => void;
  constructor(app: App, private heading: string, private message: string, private action: string) {
    super(app);
  }
  ask(): Promise<boolean> {
    return new Promise((r) => {
      this.resolve = r;
      this.open();
    });
  }
  onOpen() {
    const { contentEl } = this;
    contentEl.createEl("h2", { text: this.heading });
    contentEl.createEl("p", { text: this.message });
    new Setting(contentEl)
      .addButton((b) => b.setButtonText("Cancel").onClick(() => this.done(false)))
      .addButton((b) => b.setButtonText(this.action).setWarning().onClick(() => this.done(true)));
  }
  private done(ok: boolean) {
    this.resolve?.(ok);
    this.resolve = undefined;
    this.close();
  }
  onClose() {
    this.resolve?.(false);
    this.contentEl.empty();
  }
}
