// HTTP client for the sync protocol. The transport is injected: Obsidian's
// requestUrl in the plugin (no CORS limits on mobile), fetch in tests.

import {
  BuildStatus, CommitResponse, Info, Manifest, PROTOCOL_VERSION, ProtocolError, SiteInfo, SyncResponse, WireError,
} from "./protocol";

export interface HttpRequest {
  url: string;
  method: string;
  headers: Record<string, string>;
  body?: string | ArrayBuffer;
  contentType?: string;
}

export interface HttpResponse {
  status: number;
  text: string;
}

export type Transport = (req: HttpRequest) => Promise<HttpResponse>;

export class Client {
  readonly base: string;

  constructor(serverURL: string, private site: string, private token: string, private transport: Transport) {
    let u: URL;
    try {
      u = new URL(serverURL);
    } catch {
      throw new Error(`Invalid server URL: ${serverURL}`);
    }
    const local = ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname);
    if (u.protocol !== "https:" && !(u.protocol === "http:" && local)) {
      throw new Error("The server URL must use https:// (plain http is only allowed for localhost).");
    }
    this.base = serverURL.replace(/\/+$/, "");
  }

  private siteURL(...parts: string[]): string {
    return [this.base + "/api/v1/sites", this.site, ...parts].map((p, i) => (i === 0 ? p : encodeURIComponent(p))).join("/");
  }

  private async call<T>(method: string, url: string, body?: string | ArrayBuffer, contentType?: string): Promise<T> {
    const headers: Record<string, string> = { "Kvist-Protocol": String(PROTOCOL_VERSION) };
    if (this.token) headers["Authorization"] = "Bearer " + this.token;
    const res = await this.transport({ url, method, headers, body, contentType });
    if (res.status >= 300) {
      let err: WireError | undefined;
      try {
        err = JSON.parse(res.text)?.error;
      } catch {
        // not JSON
      }
      throw new ProtocolError(res.status, err?.code ?? "http_" + res.status, err?.message ?? `HTTP ${res.status}`, err?.details);
    }
    return (res.text ? JSON.parse(res.text) : undefined) as T;
  }

  info(): Promise<Info> {
    return this.call("GET", this.base + "/api/v1/info");
  }
  siteInfo(): Promise<SiteInfo> {
    return this.call("GET", this.siteURL());
  }
  startSync(m: Manifest): Promise<SyncResponse> {
    return this.call("POST", this.siteURL("syncs"), JSON.stringify(m), "application/json");
  }
  putBlob(syncId: string, hash: string, data: ArrayBuffer): Promise<void> {
    return this.call("PUT", this.siteURL("syncs", syncId, "blobs", hash), data, "application/octet-stream");
  }
  commit(syncId: string, force: boolean): Promise<CommitResponse> {
    return this.call("POST", this.siteURL("syncs", syncId, "commit"), JSON.stringify({ force }), "application/json");
  }
  buildStatus(id: string, waitSeconds: number): Promise<BuildStatus> {
    return this.call("GET", this.siteURL("builds", id) + (waitSeconds > 0 ? `?wait=${waitSeconds}s` : ""));
  }

  /** Follows a build, and whatever superseded it, until it is done. */
  async waitBuild(id: string, maxRounds = 10): Promise<BuildStatus> {
    for (let i = 0; i < maxRounds; i++) {
      const st = await this.buildStatus(id, 30);
      if (st.state === "superseded" && st.superseded_by) {
        id = st.superseded_by;
        continue;
      }
      if (st.state === "succeeded" || st.state === "failed") return st;
    }
    throw new Error("The build is taking long; check again later.");
  }
}
