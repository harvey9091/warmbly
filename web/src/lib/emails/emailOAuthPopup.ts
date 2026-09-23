// Drives the mailbox OAuth popup outside AddEmailModal (the reconnect flow in
// the mailbox drawer). Opens the provider authorization URL in a centered
// popup; the backend's /addresses/<provider>/callback page postMessages
// {type:"email_oauth_callback", code, state} back to this opener; we resolve
// with them so the caller can finish the handshake.

import { API_URL, APP_URL } from "@/lib/information";

export interface EmailOAuthPopupResult {
    code: string;
    state: string;
}

interface EmailOAuthCallbackMessage {
    type: "email_oauth_callback";
    provider: string;
    code: string;
    state: string;
    error: string;
}

// originOf normalises a configured base URL to a bare origin. APP_URL and
// API_URL may carry a trailing slash or a path; event.origin never does.
function originOf(value: string | undefined): string | null {
    if (!value) return null;
    try {
        return new URL(value, window.location.href).origin;
    } catch {
        return null;
    }
}

// The bridge page is served by the API so the registered redirect_uri stays
// stable, which means event.origin can be API_URL's origin on split-domain
// deployments. The real replay protection is the single-use state match.
function allowedCallbackOrigins(): string[] {
    return [originOf(APP_URL), originOf(API_URL), window.location.origin].filter(
        (o): o is string => Boolean(o),
    );
}

export function openEmailOAuthPopup(authUrl: string, expectedState: string): Promise<EmailOAuthPopupResult> {
    return new Promise((resolve, reject) => {
        const width = 520;
        const height = 640;
        const left = window.screenX + Math.max(0, (window.outerWidth - width) / 2);
        const top = window.screenY + Math.max(0, (window.outerHeight - height) / 2);
        const popup = window.open(
            authUrl,
            "warmbly_email_oauth",
            `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=yes`,
        );
        if (!popup) {
            reject(new Error("Popup blocked. Allow popups for this site and try again."));
            return;
        }
        popup.focus();

        let settled = false;
        const cleanup = () => {
            window.removeEventListener("message", onMessage);
            window.clearInterval(closedTimer);
        };

        const onMessage = (event: MessageEvent) => {
            if (event.origin && !allowedCallbackOrigins().includes(event.origin)) return;
            const data = event.data as EmailOAuthCallbackMessage | undefined;
            if (!data || data.type !== "email_oauth_callback") return;
            // An admin grant state never belongs to a mailbox re-authorization.
            if (data.state !== expectedState || data.state.startsWith("mac_") || data.state.startsWith("gac_")) return;
            settled = true;
            cleanup();
            try {
                popup.close();
            } catch {
                /* ignore */
            }
            if (data.error) {
                reject(new Error(data.error === "access_denied" ? "Authorization was cancelled." : `Provider error: ${data.error}`));
                return;
            }
            if (data.code) {
                resolve({ code: data.code, state: data.state });
                return;
            }
            reject(new Error("Authorization was cancelled."));
        };

        window.addEventListener("message", onMessage);

        // Detect a manually-closed popup so the caller's promise doesn't hang.
        const closedTimer = window.setInterval(() => {
            if (popup.closed && !settled) {
                cleanup();
                reject(new Error("Authorization window was closed before finishing."));
            }
        }, 600);
    });
}
