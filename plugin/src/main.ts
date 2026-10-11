// The kvist Obsidian plugin: decides what is public, pushes it to a kvist
// server, and reports what stays private. Uses only the Obsidian API, so it
// works on desktop and mobile.

import {
  App, ItemView, Menu, Modal, Notice, Platform, Plugin, PluginSettingTab, Setting, TAbstractFile, TFile,
  WorkspaceLeaf, getAllTags, getLanguage, normalizePath, requestUrl,
} from "obsidian";
import { Client, Transport } from "./client";
import { HashCache, HashEntry } from "./hash";
import { language, setLanguage, t } from "./i18n";
import { ProtocolError, Regression, Rules, SETTINGS_NOTE_NAME, SITE_NOTE_PATH, Warning } from "./protocol";
import { DeviceState, PublishResult, Publisher, StaleCancelled } from "./publisher";
import { evaluate, isNote, isSettingsNote } from "./rules";
import { LeakItem, NoteMeta, VaultFile, VaultLike, findSettingsNote } from "./scan";
import { noteURL } from "./slug";

const VIEW_REPORT = "kvist-report";

/** The properties a new settings note starts with, before its text (settingsNote.body). Empty ones are ignored. */
const SETTINGS_PROPERTIES = `---
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
    setLanguage(obsidianLanguage());
    this.shared = Object.assign({}, DEFAULT_SHARED, await this.loadData());
    this.local = this.loadLocal();
    this.cache = new HashCache(this.local.hashes);

    this.statusEl = this.addStatusBarItem();
    this.statusEl.addClass("mod-clickable", "kvist-status");
    this.statusEl.onClickEvent(() => this.openReport());
    this.setStatus("idle", "kvist");

    this.registerView(VIEW_REPORT, (leaf) => new ReportView(leaf, this));
    this.addSettingTab(new KvistSettingTab(this.app, this));

    this.addCommand({ id: "publish-now", name: t("command.publish"), callback: () => this.publish(false) });
    this.addCommand({
      id: "toggle-publish",
      name: t("command.toggle"),
      checkCallback: (checking) => {
        const f = this.app.workspace.getActiveFile();
        if (!f || f.extension !== "md") return false;
        if (!checking) void this.togglePublish(f);
        return true;
      },
    });
    this.addCommand({ id: "open-report", name: t("command.report"), callback: () => this.openReport() });
    this.addCommand({ id: "settings-note", name: t("command.settingsNote"), callback: () => this.openSettingsNote() });
    this.addCommand({
      id: "open-published",
      name: t("command.openPublished"),
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
          item.setTitle(t(published ? "menu.unpublish" : "menu.publish")).setIcon("globe").onClick(() => this.togglePublish(file)),
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
    this.statusEl.setAttribute("aria-label", t("status.label", { status: text }));
    this.statusEl.toggleClass("kvist-warn", s === "warnings" || s === "stale");
    this.statusEl.toggleClass("kvist-error", s === "error");
    this.app.workspace.getLeavesOfType(VIEW_REPORT).forEach((l) => (l.view as ReportView).render());
  }

  // ---- publishing ----

  schedule() {
    if (!this.local.autoPublish || !this.configured() || this.status === "stale") return;
    if (this.timer) window.clearTimeout(this.timer);
    if (!this.running) this.setStatus("pending", t("status.pending"));
    this.timer = window.setTimeout(() => void this.publish(true), Math.max(5, this.local.debounceSeconds) * 1000);
  }

  async publish(auto: boolean) {
    if (!this.configured()) {
      new Notice(t("notice.notConfigured"));
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
      this.setStatus("publishing", t("status.publishing"));
      const res = await pub.publish({
        skipIfUnchanged: auto,
        waitForBuild: true,
        onProgress: (m) => this.setStatus("publishing", m),
        confirmStale: auto ? undefined : (msg, regs) => new StaleModal(this.app, msg, regs).ask(),
      });
      this.saveLocal();
      this.lastResult = res;
      this.rules = res.site.rules;
      this.lastLeaks = res.scan.leaks;
      if (!res.skipped) this.lastWarnings = [...res.warnings, ...(res.build?.warnings ?? [])];
      const time = new Date().toLocaleTimeString(language(), { hour: "2-digit", minute: "2-digit" });
      if (res.build?.state === "failed") {
        this.setStatus("error", t("status.buildFailed"));
        new Notice(t("notice.buildFailed", { error: res.build.error ?? "" }));
      } else if (this.lastWarnings.length) {
        this.setStatus("warnings", t("status.publishedWarnings", { time, count: this.lastWarnings.length }));
      } else {
        this.setStatus("ok", t("status.published", { time }));
      }
      if (!auto) {
        const n = res.scan.files.filter((f) => isNote(f.path)).length;
        new Notice(res.unchanged ? t("notice.upToDate", { count: n }) : t("notice.published", { count: n, uploaded: res.uploaded }));
      }
    } catch (e) {
      if (e instanceof StaleCancelled || (e instanceof ProtocolError && e.code === "stale_state")) {
        this.setStatus("stale", t("status.stale"));
        new Notice(t("notice.stale"));
      } else {
        this.setStatus("error", t("status.failed"));
        new Notice(t("notice.error", { error: errorText(e) }));
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
      new Notice(t("notice.unreachable", { error: errorText(e) }));
    }
    return this.rules;
  }

  async togglePublish(file: TFile) {
    if (isSettingsNote(file.path)) {
      new Notice(t("notice.isSettingsNote"));
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
      const reason = reasonText(after.reason);
      new Notice(t(after.published ? "notice.staysPublic" : "notice.staysPrivate", { name: file.basename, reason }));
    } else {
      new Notice(t(after.published ? "notice.willPublish" : "notice.willUnpublish", { name: file.basename }));
    }
  }

  /** Opens the settings note (_site.md), creating it from a template if there is none. */
  async openSettingsNote() {
    let path: string | undefined;
    try {
      path = findSettingsNote(this.app.vault.getMarkdownFiles().map((f) => f.path));
    } catch (e) {
      new Notice(t("notice.error", { error: errorText(e) }));
      return;
    }
    if (!path) {
      // Next to the public notes if the rules name a folder, else at the top.
      const rules = await this.ensureRules();
      const folder = (rules?.always_public_folders ?? []).map((f) => f.replace(/^\/+|\/+$/g, "")).find((f) => f !== "") ?? "";
      if (folder && !this.app.vault.getAbstractFileByPath(folder)) await this.app.vault.createFolder(folder);
      path = normalizePath(folder ? `${folder}/${SETTINGS_NOTE_NAME}` : SETTINGS_NOTE_NAME);
      await this.app.vault.create(path, SETTINGS_PROPERTIES + t("settingsNote.body"));
      new Notice(t("notice.settingsNoteCreated"));
    }
    const file = this.app.vault.getAbstractFileByPath(path);
    if (file instanceof TFile) await this.app.workspace.getLeaf(false).openFile(file);
  }

  openPublished(file: TFile) {
    const base = this.lastResult?.site.base_url;
    if (!base) {
      new Notice(t("notice.publishFirst"));
      return;
    }
    const d = this.decide(file);
    if (!d?.published) {
      new Notice(t("notice.notPublished"));
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

/** Obsidian's interface language: getLanguage() from 1.8.7, before that its localStorage key. */
function obsidianLanguage(): string {
  if (typeof getLanguage === "function") return getLanguage();
  return window.localStorage.getItem("language") ?? "en";
}

/** Why a rule overrides the publish property, in the plugin's language. */
function reasonText(reason: string): string {
  switch (reason) {
    case "excluded_folder":
      return t("reason.excluded_folder");
    case "private_tag":
      return t("reason.private_tag");
  }
  return reason;
}

/** An error for a notice: server errors the user can act on in the plugin's language, else the message. */
function errorText(e: unknown): string {
  if (e instanceof ProtocolError) {
    switch (e.code) {
      case "unauthorized":
        return t("error.unauthorized");
      case "protocol_unsupported":
        return t("error.protocol_unsupported");
    }
  }
  return e instanceof Error ? e.message : String(e);
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
    if (!(f instanceof TFile)) throw new Error(t("error.disappeared", { path }));
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
    contentEl.createEl("h2", { text: t("stale.title") });
    contentEl.createEl("p", { text: this.message });
    contentEl.createEl("p", { text: t("stale.explain") });
    const ul = contentEl.createEl("ul");
    for (const r of this.regressions.slice(0, 30)) {
      ul.createEl("li", { text: t(r.kind === "older" ? "stale.older" : "stale.missing", { path: r.path }) });
    }
    if (this.regressions.length > 30) ul.createEl("li", { text: t("stale.more", { count: this.regressions.length - 30 }) });
    new Setting(contentEl)
      .addButton((b) => b.setButtonText(t("button.cancel")).onClick(() => this.done(false)))
      .addButton((b) => b.setButtonText(t("stale.publishAnyway")).setWarning().onClick(() => this.done(true)));
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
    return t("report.title");
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
    el.createEl("p", { text: this.plugin.statusText || t("report.notPublished"), cls: "kvist-report-status" });
    new Setting(el).addButton((b) => b.setButtonText(t("command.publish")).setCta().onClick(() => this.plugin.publish(false)));

    const res = this.plugin.lastResult;
    if (!res) {
      el.createEl("p", { text: t("report.publishOnce"), cls: "setting-item-description" });
      return;
    }
    const notes = [...res.scan.decisions.entries()].filter(([, d]) => d.published).length;
    el.createEl("p", { text: t("report.public", { count: notes, files: res.scan.files.length - notes }) });

    this.section(el, t("report.links"), t("report.linksDesc"),
      this.plugin.lastLeaks.filter((l) => l.kind === "unpublished_link"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, t("report.embeds"), t("report.embedsDesc"),
      this.plugin.lastLeaks.filter((l) => l.kind === "unpublished_embed"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, t("report.attachments"), t("report.attachmentsDesc"),
      this.plugin.lastLeaks.filter((l) => l.kind === "excluded_attachment"), (l) => [l.from, `→ ${l.target}`]);
    this.section(el, t("report.server"), t("report.serverDesc"),
      this.plugin.lastWarnings, (w) => [w.path === SITE_NOTE_PATH ? res.scan.settingsNote ?? "" : w.path ?? "", w.message]);
  }

  private section<T>(el: HTMLElement, title: string, desc: string, items: T[], row: (t: T) => [string, string]) {
    if (!items.length) return;
    el.createEl("h5", { text: t("report.section", { title, count: items.length }) });
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
      text: t("settings.intro"),
      cls: "setting-item-description",
    });
    new Setting(containerEl).setName(t("settings.serverURL")).setDesc(t("settings.serverURLDesc"))
      .addText((t) => t.setPlaceholder("https://").setValue(p.shared.serverURL).onChange(async (v) => {
        p.shared.serverURL = v.trim();
        p.rules = undefined;
        await p.saveShared();
      }));
    new Setting(containerEl).setName(t("settings.site")).setDesc(t("settings.siteDesc"))
      .addText((t) => t.setValue(p.shared.site).onChange(async (v) => {
        p.shared.site = v.trim();
        p.rules = undefined;
        await p.saveShared();
      }));

    new Setting(containerEl).setName(t("settings.siteSettings"))
      .setDesc(t("settings.siteSettingsDesc", { name: SETTINGS_NOTE_NAME }))
      .addButton((b) => b.setButtonText(t("settings.openSettingsNote")).onClick(() => p.openSettingsNote()));

    containerEl.createEl("h3", { text: t("settings.device") });
    containerEl.createEl("p", {
      text: t("settings.deviceDesc"),
      cls: "setting-item-description",
    });
    new Setting(containerEl).setName(t("settings.token")).setDesc(t("settings.tokenDesc"))
      .addText((t) => {
        t.inputEl.type = "password";
        t.setValue(p.local.token).onChange((v) => {
          p.local.token = v.trim();
          p.saveLocal();
        });
      });
    new Setting(containerEl).setName(t("settings.deviceName")).setDesc(t("settings.deviceNameDesc"))
      .addText((t) => t.setValue(p.local.deviceName).onChange((v) => {
        p.local.deviceName = v.trim();
        p.saveLocal();
      }));
    new Setting(containerEl).setName(t("settings.autoPublish"))
      .setDesc(t("settings.autoPublishDesc"))
      .addToggle((t) => t.setValue(p.local.autoPublish).onChange((v) => {
        p.local.autoPublish = v;
        p.saveLocal();
      }));
    new Setting(containerEl).setName(t("settings.debounce"))
      .addText((t) => t.setValue(String(p.local.debounceSeconds)).onChange((v) => {
        const n = parseInt(v, 10);
        if (n >= 5) {
          p.local.debounceSeconds = n;
          p.saveLocal();
        }
      }));
    new Setting(containerEl).setName(t("settings.test")).addButton((b) => b.setButtonText(t("settings.testButton")).onClick(async () => {
      try {
        const c = new Client(p.shared.serverURL, p.shared.site, p.local.token, obsidianTransport);
        const info = await c.siteInfo();
        p.rules = info.rules;
        new Notice(t("notice.connected", { site: info.site, url: info.base_url }));
      } catch (e) {
        new Notice(t("notice.error", { error: errorText(e) }));
      }
    }));

    containerEl.createEl("h3", { text: t("settings.reset") });
    new Setting(containerEl).setName(t("settings.resetName"))
      .setDesc(t("settings.resetDesc"))
      .addButton((b) => b.setButtonText(t("settings.resetButton")).setWarning().onClick(async () => {
        if (p.publishing()) {
          new Notice(t("notice.waitForPublish"));
          return;
        }
        const ok = await new ConfirmModal(this.app, t("settings.resetConfirmTitle"), t("settings.resetConfirm"), t("settings.resetButton")).ask();
        if (!ok) return;
        await p.resetSettings();
        new Notice(t("notice.reset"));
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
      .addButton((b) => b.setButtonText(t("button.cancel")).onClick(() => this.done(false)))
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
