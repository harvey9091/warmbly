// Browser error reporting.
//
// The DSN is the operator's choice, not a requirement of the software: the
// dashboard image is the same for the hosted service and for a self-host, so a
// literal DSN in the bundle would make every self-hosted install report its
// users' errors, URLs and IPs to somebody else's Sentry. It comes from the
// container-injected runtime config instead, and an unset DSN means the SDK is
// never initialised, so nothing is ever sent anywhere.
//
// The SDK is imported statically rather than lazily so that its global handlers
// are installed before the first render: a broken deploy fails during boot, and
// a chunk still in flight would miss exactly that error. Not initialising it
// costs a self-hoster some dead bundle weight and zero network calls.
import * as Sentry from "@sentry/react";
import { SENTRY_DSN } from "./information";

let reporting = false;

// initErrorReporting is called once, before the app renders.
export function initErrorReporting(): void {
    if (!SENTRY_DSN) return;

    Sentry.init({
        dsn: SENTRY_DSN,
        sendDefaultPii: true,
        environment: import.meta.env.MODE,
    });
    reporting = true;
}

// captureException reports an error the app handled itself. A no-op when no DSN
// is configured.
export function captureException(error: unknown): void {
    if (!reporting) return;
    Sentry.captureException(error);
}
