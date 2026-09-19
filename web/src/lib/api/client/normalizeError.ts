import axios from "axios";
import { AuthError } from "@/lib/errors/auth";

export interface AppError {
    error: string;
    message: string;
    status?: number;
    redirect?: boolean;
    /** Stable machine-readable code from the API, for branching on a specific
     *  condition rather than matching on human-readable text. */
    code?: string;
    /** Correlation id the API already returns, so a user can quote it and an
     *  operator can find the matching server-side log line. */
    request_id?: string;
    /** Seconds the API asked us to wait, from Retry-After on a 429 or 503.
     *  Surfaced so a message can name the wait instead of guessing at one. */
    retry_after?: number;
}

/** The API's error envelope, or as much of it as arrived.
 *
 *  A response body is not always ours: a proxy in front of the API answers a
 *  502 with an HTML page, a gateway timeout can carry nothing at all, and a
 *  request for a file gets a Blob. Reading `.error` straight off any of those
 *  produced either `undefined` in the message or, for a null body, a TypeError
 *  thrown from inside the error handler itself. */
function envelope(data: unknown): { error?: string; message?: string; code?: string; request_id?: string } {
    if (!data || typeof data !== "object" || Array.isArray(data)) return {};
    return data as { error?: string; message?: string; code?: string; request_id?: string };
}

/** What to say when the response carried no message of its own. Each one names
 *  what the reader can do next rather than restating the status. */
function fallback(status: number): string {
    switch (status) {
        case 401: return "Your session has expired. Sign in again to continue.";
        case 403: return "You don't have permission to do that.";
        case 404: return "That item no longer exists. It may have been deleted.";
        case 409: return "Someone else changed this first. Reload the page and try again.";
        case 413: return "That file is too large to upload.";
        case 429: return "You're going a little fast. Wait a moment and try again.";
        case 502:
        case 504: return "The server didn't answer in time. Try again in a moment.";
        case 503: return "Warmbly is temporarily unavailable. Try again in a moment.";
    }
    if (status >= 500) return "Something went wrong on our end. Try again in a moment.";
    return "The request couldn't be completed. Check the details and try again.";
}

/** Retry-After is either a number of seconds or an HTTP date. */
function retryAfter(header: unknown): number | undefined {
    if (typeof header !== "string" || header === "") return undefined;
    const seconds = Number(header);
    if (Number.isFinite(seconds)) return Math.max(0, Math.round(seconds));
    const at = Date.parse(header);
    if (Number.isNaN(at)) return undefined;
    return Math.max(0, Math.round((at - Date.now()) / 1000));
}

export function normalizeError(error: unknown): AppError {
    if (error instanceof AuthError) {
        return {
            error: "Authentication Required",
            message: error.message,
            status: 401,
            redirect: true,
        };
    }

    if (axios.isAxiosError(error)) {
        if (!error.response) {
            // No answer at all: offline, DNS, TLS, CORS, a cancelled
            // navigation, or a timeout we set ourselves. The browser's own
            // wording for all of these is "Network Error", which reads as a
            // fault in the app rather than in the connection.
            if (error.code === "ECONNABORTED" || error.code === "ETIMEDOUT") {
                return {
                    error: "Timed Out",
                    message: "That took too long to answer. Try again in a moment.",
                };
            }
            if (typeof navigator !== "undefined" && navigator.onLine === false) {
                return {
                    error: "Offline",
                    message: "You're offline. Reconnect and try again; nothing was saved.",
                };
            }
            return {
                error: "Network Error",
                message: "We couldn't reach Warmbly. Check your connection and try again.",
            };
        }

        const status = error.response.status;
        const data = envelope(error.response.data);
        const wait = retryAfter(error.response.headers?.["retry-after"]);

        let message = data.message || fallback(status);
        // A wait the server named beats one the reader has to guess at.
        if (wait !== undefined && !data.message && (status === 429 || status === 503)) {
            message = wait > 60
                ? `Too many requests. Try again in about ${Math.ceil(wait / 60)} minutes.`
                : `Too many requests. Try again in ${Math.max(wait, 1)} seconds.`;
        }

        return {
            error: data.error || (status === 401 ? "Unauthorized" : "Unknown Error"),
            message,
            status,
            ...(status === 401 ? { redirect: true } : {}),
            code: data.code,
            request_id: data.request_id,
            ...(wait !== undefined ? { retry_after: wait } : {}),
        };
    }

    // Anything that is not an axios failure still reached a catch block that
    // has to render something. A thrown Error at least knows what it was.
    if (error instanceof Error && error.message) {
        return { error: error.name || "Unknown Error", message: error.message };
    }

    return {
        error: "Unknown Error",
        message: "Something went wrong. Try again, and reload the page if it keeps happening.",
    };
}
