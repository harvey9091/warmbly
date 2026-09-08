// Connection security for a mailbox leg. "tls" is negotiated before the
// greeting, "starttls" upgrades in-band after it, and "none" is no encryption
// at all, which is only legal against a loopback host on a self-hosted
// instance. Mirrors models.MailSecurity* in Go.
export type MailSecurity = "tls" | "starttls" | "none";

export default interface Service {
    username: string;
    password: string;
    host: string;
    port: number;
    /** Omitted means "infer from the port", matching the backend. */
    security?: MailSecurity;
}

/** Conventional SMTP default: 465 is implicit TLS, everything else STARTTLS. */
export function defaultSmtpSecurity(port: number): MailSecurity {
    return port === 465 ? "tls" : "starttls";
}

/** Conventional IMAP default: 143 is STARTTLS, everything else implicit TLS. */
export function defaultImapSecurity(port: number): MailSecurity {
    return port === 143 ? "starttls" : "tls";
}

/** Any routable TCP port; the security mode carries how to connect. */
export function validPort(port: number): boolean {
    return Number.isInteger(port) && port > 0 && port <= 65535;
}

/**
 * Whether a host addresses this machine, the only place `none` is allowed.
 *
 * Literals only, matching models.LoopbackMailHost in Go: a name that resolves
 * to 127.0.0.1 today can resolve elsewhere at dial time, so the backend and
 * the worker both refuse anything but a literal and the form must not offer
 * what they would refuse.
 */
export function isLoopbackHost(host: string): boolean {
    const h = host.trim().toLowerCase();
    if (h === "localhost") return true;
    const bare = h.startsWith("[") && h.endsWith("]") ? h.slice(1, -1) : h;
    if (/^127(\.\d{1,3}){3}$/.test(bare)) {
        return bare.split(".").every((o) => Number(o) <= 255);
    }
    return bare === "::1" || bare === "0:0:0:0:0:0:0:1";
}

/**
 * Whether the unencrypted mode is offerable at all: a loopback host on a
 * self-hosted instance. On the hosted product the worker is never the
 * customer's machine, so there is no local relay for it to reach.
 */
export function allowsNoEncryption(host: string, selfHosted: boolean): boolean {
    return selfHosted && isLoopbackHost(host);
}
