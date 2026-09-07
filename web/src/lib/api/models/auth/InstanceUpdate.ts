/**
 * The platform admin's view of updates, served by GET /admin/instance/update.
 * Mirrors internal/app/updates.State. Only a platform admin can read it; the
 * member-facing summary is InstanceVersion. Timestamps are Date objects:
 * Request revives ISO strings on every response.
 */
export interface InstanceCheckout {
    branch: string;
    detached: boolean;
    commit: string;
    describe: string;
    remote_commit: string;
    behind: number;
    dirty: boolean;
    fetched_at: Date;
    fetch_error?: string;
}

export type UpdateJobStatus = "running" | "succeeded" | "failed";

export interface UpdateJob {
    id: string;
    status: UpdateJobStatus;
    target: string;
    step: string;
    started_at: Date;
    finished_at?: Date;
    error?: string;
    from_commit: string;
    to_commit?: string;
    // Absent unless fetched with log=1.
    log?: string[] | null;
}

export interface InstanceUpdater {
    configured: boolean;
    status: "off" | "ok" | "unreachable";
    error?: string;
    mode?: "compose" | "command";
    repo_dir?: string;
    checkout?: InstanceCheckout;
    job?: UpdateJob;
    last_job?: UpdateJob;
}

export default interface InstanceUpdate {
    running: { version: string; commit?: string; built_at?: Date };
    latest?: { tag: string; name?: string; html_url?: string; published_at?: Date; channel: string };
    update_available: boolean;
    reason?: "release" | "commits";
    checked_at?: Date;
    check_error?: string;
    enabled: boolean;
    interval: string;
    channel: string;
    repo: string;
    updater: InstanceUpdater;
}
