// The publish flow: handshake, rules, scan, sync, upload, commit, build.

import { Client } from "./client";
import { HashCache } from "./hash";
import { BuildStatus, PROTOCOL_VERSION, ProtocolError, Regression, SiteInfo, Warning } from "./protocol";
import { ScanResult, VaultLike, scan } from "./scan";

/** Device-local state (never synced between devices). */
export interface DeviceState {
  clientId: string;
  baseRevision: string;
  /** Signature of the last committed manifest, to skip no-op pushes. */
  lastSignature?: string;
}

export interface PublishOptions {
  force?: boolean;
  /** Asked when the push would restore older content; resolve true to go ahead. */
  confirmStale?: (message: string, regressions: Regression[]) => Promise<boolean>;
  /** Skip the request entirely if nothing changed since the last push. */
  skipIfUnchanged?: boolean;
  waitForBuild?: boolean;
  onProgress?: (msg: string) => void;
}

export interface PublishResult {
  scan: ScanResult;
  site: SiteInfo;
  revision: string;
  unchanged: boolean;
  skipped?: boolean;
  uploaded: number;
  warnings: Warning[];
  build?: BuildStatus;
}

export class StaleCancelled extends Error {
  constructor(public regressions: Regression[]) {
    super("Publishing was cancelled: the server has newer content.");
    this.name = "StaleCancelled";
  }
}

const RETRY = new Set(["revision_changed", "rules_changed", "sync_expired"]);

export class Publisher {
  constructor(
    private client: Client,
    private vault: VaultLike,
    private cache: HashCache,
    private state: DeviceState,
    private saveState: (s: DeviceState) => void,
    private clientName: string,
    private platform: string,
    private version: string,
  ) {}

  async publish(opts: PublishOptions = {}): Promise<PublishResult> {
    const progress = opts.onProgress ?? (() => {});
    const info = await this.client.info();
    if (PROTOCOL_VERSION < info.protocol.min || PROTOCOL_VERSION > info.protocol.max) {
      const side = PROTOCOL_VERSION < info.protocol.min ? "the kvist plugin" : "the kvist server";
      throw new Error(`The server speaks protocol ${info.protocol.min}–${info.protocol.max}; update ${side}.`);
    }
    let lastErr: unknown;
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return await this.once(opts, progress);
      } catch (e) {
        if (e instanceof ProtocolError && RETRY.has(e.code)) {
          lastErr = e;
          continue;
        }
        throw e;
      }
    }
    throw lastErr;
  }

  private async once(opts: PublishOptions, progress: (m: string) => void): Promise<PublishResult> {
    const site = await this.client.siteInfo();
    progress("Checking notes…");
    const result = await scan(this.vault, site.rules, this.cache);
    if (result.files.length > site.limits.max_files) {
      throw new Error(`${result.files.length} files to publish; the server allows ${site.limits.max_files}.`);
    }
    const big = result.files.find((f) => f.size > site.limits.max_file_size);
    if (big) throw new Error(`${big.path} is too large to publish (limit ${Math.round(site.limits.max_file_size / 1048576)} MB).`);

    const signature = site.rules_hash + "\n" + result.files.map((f) => f.path + "\0" + f.hash).join("\n");
    if (opts.skipIfUnchanged && !opts.force && signature === this.state.lastSignature && site.revision === this.state.baseRevision) {
      return { scan: result, site, revision: site.revision, unchanged: true, skipped: true, uploaded: 0, warnings: [] };
    }

    const sync = await this.client.startSync({
      base_revision: this.state.baseRevision,
      rules_hash: site.rules_hash,
      client: { id: this.state.clientId, name: this.clientName, platform: this.platform, version: this.version },
      files: result.files,
    });
    await this.upload(sync.sync_id, sync.missing, result, progress);

    progress("Publishing…");
    let commit;
    try {
      commit = await this.client.commit(sync.sync_id, !!opts.force);
    } catch (e) {
      if (!(e instanceof ProtocolError && e.code === "stale_state")) throw e;
      const regressions = (e.details as Regression[]) ?? [];
      const ok = opts.confirmStale ? await opts.confirmStale(e.message, regressions) : false;
      if (!ok) throw new StaleCancelled(regressions);
      commit = await this.client.commit(sync.sync_id, true);
    }
    this.state.baseRevision = commit.revision;
    this.state.lastSignature = signature;
    this.saveState(this.state);

    const res: PublishResult = {
      scan: result, site, revision: commit.revision, unchanged: !!commit.unchanged,
      uploaded: sync.missing.length, warnings: commit.warnings ?? [],
    };
    if (opts.waitForBuild && commit.build_id) {
      progress("Building the site…");
      res.build = await this.client.waitBuild(commit.build_id);
    }
    return res;
  }

  private async upload(syncId: string, missing: string[], result: ScanResult, progress: (m: string) => void) {
    if (!missing.length) return;
    const byHash = new Map<string, string>();
    for (const f of result.files) byHash.set(f.hash, f.path);
    let next = 0;
    let done = 0;
    const worker = async () => {
      while (next < missing.length) {
        const hash = missing[next++];
        const path = byHash.get(hash);
        const read = path ? result.content.get(path) : undefined;
        if (!path || !read) throw new Error(`The server asked for an unknown file (${hash}).`);
        await this.client.putBlob(syncId, hash, await read());
        done++;
        progress(`Uploading ${done}/${missing.length}…`);
      }
    };
    await Promise.all(Array.from({ length: Math.min(4, missing.length) }, worker));
  }
}
