// Mirrors internal/pkg/displayname: names reach other people's inboxes, where
// anything shaped like an address is turned into a link. The server is the
// authority; this only lets the form say so before the request.

export type NameKind = "person" | "workspace";

export const PERSON_NAME_MAX = 50;
export const WORKSPACE_NAME_MAX = 64;
const MAX_COMBINING_RUN = 3;

const SCHEME =
    /(?:^|[^\p{L}\p{N}])(?:https?|ftps?|mailto|tel|sms|callto|skype|javascript|data|file|news|irc|xmpp|ssh|git|ws|wss)\s*:/iu;
const DOMAIN = /[\p{L}\p{N}-]\.(?:\p{L}{2,}|xn--)/u;
const IPV4 = /\d{1,3}(?:\.\d{1,3}){3}/;
const FORBIDDEN_CHAR = /[\p{Cc}\p{Cf}\p{Co}\p{Cs}\p{Zl}\p{Zp}<>`\\]/u;
const COMBINING = /[\p{Mn}\p{Me}]/u;

/** Trims and collapses spaces, the form the server stores. */
export function normalizeName(value: string): string {
    return value.normalize("NFC").trim().replace(/[\p{Zs} ]+/gu, " ");
}

function looksLikeLink(value: string): boolean {
    const f = value.normalize("NFKC").toLowerCase().replace(/[。｡]/g, ".");
    if (f.includes("@") || f.includes("//") || f.includes("www.")) return true;
    return SCHEME.test(f) || DOMAIN.test(f) || IPV4.test(f);
}

/** The reason a name would be refused, or null when it is accepted. */
export function nameError(label: string, value: string, kind: NameKind, optional = false): string | null {
    const name = normalizeName(value);
    if (!name) return optional ? null : `${label} is required.`;
    const max = kind === "workspace" ? WORKSPACE_NAME_MAX : PERSON_NAME_MAX;
    if ([...name].length > max) return `${label} must be ${max} characters or less.`;
    let combining = 0;
    for (const ch of name) {
        if (FORBIDDEN_CHAR.test(ch)) return `${label} contains characters that are not allowed.`;
        combining = COMBINING.test(ch) ? combining + 1 : 0;
        if (combining > MAX_COMBINING_RUN) return `${label} contains characters that are not allowed.`;
    }
    if (looksLikeLink(name)) return `${label} cannot contain a link, web address or email address.`;
    const letter = kind === "workspace" ? /[\p{L}\p{N}]/u : /\p{L}/u;
    if (!letter.test(name)) {
        return kind === "workspace" ? `${label} must contain a letter or a number.` : `${label} must contain a letter.`;
    }
    return null;
}
