// Browser error reporting for the operator panel.
//
// Same rule as the dashboard: the admin image ships to self-hosters too, so a
// literal DSN here would make every self-hosted panel report its operator's
// errors and URLs to somebody else. The DSN comes from the container-injected
// runtime config, and an unset DSN means the SDK is never initialised, so
// nothing is ever sent anywhere.
import * as Sentry from "@sentry/react";
import { SENTRY_DSN, SENTRY_ENVIRONMENT, SENTRY_RELEASE } from "./env";

let reporting = false;

// initErrorReporting is called once, before the app renders.
export function initErrorReporting(): void {
    if (!SENTRY_DSN) return;

    Sentry.init({
        dsn: SENTRY_DSN,
        sendDefaultPii: true,
        environment: SENTRY_ENVIRONMENT,
        // Empty is omitted rather than sent: an event tagged with the empty
        // release matches no uploaded source map and reads as a real release.
        release: SENTRY_RELEASE || undefined,
    });
    reporting = true;
}

// captureException reports an error the app handled itself. A no-op when no DSN
// is configured.
export function captureException(error: unknown): void {
    if (!reporting) return;
    Sentry.captureException(error);
}
