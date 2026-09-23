import type { SigninMigration } from "@/lib/api/models/app/emails/MailboxSources";

/** What a group of mailboxes moves to. */
export type MigrationRoute = "grant" | "setup" | "app_password";

export function migrationRoute(group: SigninMigration, googleGrants: boolean): MigrationRoute {
    if (group.kind === "personal") return "app_password";
    if (group.grant_id) return "grant";
    return googleGrants ? "setup" : "app_password";
}
