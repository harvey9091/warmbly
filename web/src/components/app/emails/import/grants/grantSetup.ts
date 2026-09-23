// What an operator still has to set before admin grants work on this instance.
import type { GrantConfig, GrantProvider } from "@/lib/api/models/app/emails/MailboxSources";

const DEFAULT_MISSING: Record<GrantProvider, string[]> = {
    google: ["GOOGLE_WORKSPACE_DELEGATION_KEY"],
    microsoft: ["BOX_OUTLOOK_CLIENT_ID", "BOX_OUTLOOK_CLIENT_SECRET"],
};

export const MS_GRANT_PERMISSIONS = ["Mail.ReadWrite", "Mail.Send", "User.Read.All"];

export function grantMissing(config: GrantConfig | undefined, provider: GrantProvider): string[] {
    const list = provider === "google" ? config?.google_missing : config?.microsoft_missing;
    return list && list.length > 0 ? list : DEFAULT_MISSING[provider];
}
