import { API_BASE_URL } from "@/lib/information";
import type PasskeyLoginBegin from "@/lib/api/models/auth/PasskeyLoginBegin";

export default async function passkeyLoginBegin(signal?: AbortSignal): Promise<PasskeyLoginBegin> {
    // Keep this as a direct fetch instead of the shared axios wrapper. Safari's
    // WebAuthn user-gesture detection is sensitive to async wrapper layers
    // before startAuthentication().
    //
    // The signal matters because the sign-in page starts this on mount: without
    // one, navigating away mid-flight leaves the request to reject into a
    // component that is gone, which read as a failure rather than as leaving.
    const response = await fetch(`${API_BASE_URL}/auth/passkey/login/begin`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal,
    });

    if (!response.ok) {
        let message = "Couldn't start passkey sign-in.";
        let code: string | undefined;
        try {
            const body = await response.json() as { message?: string; error?: string; code?: string };
            message = body.message || body.error || message;
            code = body.code;
        } catch {
            /* keep fallback */
        }
        // Same shape as the normalized AppError the axios client throws, so a
        // caller can tell "later" from "broken" without parsing the sentence.
        const err = new Error(message) as Error & { status?: number; code?: string };
        err.status = response.status;
        err.code = code;
        throw err;
    }

    return await response.json() as PasskeyLoginBegin;
}
