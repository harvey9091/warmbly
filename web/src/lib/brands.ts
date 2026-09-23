// Logos and names for mail hosts, inbox vendors and sign-in providers; ProviderLogo draws them.
import { mailHostLabel } from "@/lib/mailHost";
import { vendorLabel } from "@/lib/api/models/app/emails/MailboxSources";

export const GOOGLE_BRAND_IDS = new Set(["google", "google_workspace", "gmail"]);
export const MICROSOFT_BRAND_IDS = new Set(["microsoft", "microsoft365", "outlook"]);
export const GENERIC_BRAND_IDS = new Set(["", "smtp", "smtp_imap", "other"]);

const HOST_LOGOS: Record<string, string> = {
    aol: "aol.png",
    fastmail: "fastmail.png",
    gmx: "gmx.png",
    godaddy: "godaddy.png",
    hostinger: "hostinger.png",
    icloud: "icloud.png",
    ionos: "ionos.svg",
    migadu: "migadu.png",
    namecheap: "namecheap.png",
    ovh: "ovh.svg",
    proton: "proton.png",
    purelymail: "purelymail.png",
    rackspace: "rackspace.png",
    yahoo: "yahoo.png",
    yandex: "yandex.png",
    zoho: "zoho.png",
};

export const VENDOR_IDS = ["inboxkit", "zapmail", "mailforge", "infraforge", "maildoso", "cheapinboxes", "scaledmail"] as const;
const VENDOR_SET = new Set<string>(VENDOR_IDS);

export function normBrand(id?: string | null): string {
    return (id ?? "").trim().toLowerCase();
}

/** The shipped image for a host or vendor id, if any. */
export function brandImage(id?: string | null): string | null {
    const k = normBrand(id);
    if (VENDOR_SET.has(k)) return `/logos/vendors/${k}.png`;
    if (HOST_LOGOS[k]) return `/logos/hosts/${HOST_LOGOS[k]}`;
    return null;
}

/** A readable name for a host, vendor or provider id. */
export function brandLabel(id?: string | null): string {
    const k = normBrand(id);
    if (k === "google") return "Google";
    if (k === "microsoft") return "Microsoft";
    if (k === "smtp" || k === "smtp_imap") return "SMTP/IMAP";
    if (VENDOR_SET.has(k)) return vendorLabel(k);
    return mailHostLabel(k) || vendorLabel(k) || k;
}
