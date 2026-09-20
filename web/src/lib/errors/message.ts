// The axios interceptor in api/client/Client.ts throws a normalized AppError,
// which is a plain object and not an Error. `err instanceof Error ? err.message
// : fallback` therefore took the fallback branch for every API failure, so the
// reason the server gave — "avatar must be a PNG or JPG", a permission, a
// request id to quote — was replaced by a generic sentence at every call site
// that used it. This reads whichever shape arrived.
export function errorMessage(err: unknown, fallback: string): string {
    if (typeof err === "object" && err !== null) {
        const message = (err as { message?: unknown }).message;
        if (typeof message === "string" && message.trim() !== "") return message;
    }
    return fallback;
}
