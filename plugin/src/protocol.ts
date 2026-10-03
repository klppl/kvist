// Wire types of the kvist sync protocol v1 (docs/protocol.md). Keep in sync
// with internal/protocol in the Go server.

export const PROTOCOL_VERSION = 1;
export const HINTS_PATH = ".kvist/links.json";
export const SITE_CONFIG_PATH = ".kvist/site.toml";

export interface Info {
  protocol: { min: number; max: number };
  server: string;
}

export interface Rules {
  always_public_folders: string[];
  exclude_folders: string[];
  public_tag: string;
  private_tag: string;
  frontmatter_key: string;
  attachment_extensions: string[];
}

export interface Limits {
  max_file_size: number;
  max_files: number;
  max_path_bytes: number;
}

export interface SiteInfo {
  site: string;
  base_url: string;
  rules: Rules;
  rules_hash: string;
  limits: Limits;
  revision: string;
}

export interface ClientInfo {
  id: string;
  name?: string;
  platform?: string;
  version?: string;
}

export interface ManifestFile {
  path: string;
  hash: string;
  size: number;
  mtime: string; // RFC 3339
}

export interface Manifest {
  base_revision: string;
  rules_hash: string;
  client: ClientInfo;
  files: ManifestFile[];
}

export interface Regression {
  path: string;
  kind: "older" | "removed";
  stored_mtime: string;
  incoming_mtime?: string;
}

export interface SyncResponse {
  sync_id: string;
  missing: string[];
  expires_at: string;
  base_check: { head: string; stale: boolean; regressions?: Regression[] };
}

export interface Warning {
  code: string;
  path?: string;
  message: string;
}

export interface CommitResponse {
  revision: string;
  unchanged?: boolean;
  build_id?: string;
  warnings: Warning[];
}

export interface BuildStatus {
  id: string;
  revision: string;
  state: "queued" | "running" | "succeeded" | "failed" | "superseded";
  superseded_by?: string;
  error?: string;
  warnings: Warning[];
}

export interface WireError {
  code: string;
  message: string;
  details?: unknown;
}

/** ProtocolError is a non-2xx answer from the server. */
export class ProtocolError extends Error {
  constructor(public status: number, public code: string, message: string, public details?: unknown) {
    super(message);
    this.name = "ProtocolError";
  }
}
