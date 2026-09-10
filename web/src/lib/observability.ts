// Browser error reporting.
//
// PostHog is the default backend and Sentry is still supported: set both and
// events go to both, set neither and nothing is reported anywhere. Neither is a
// requirement of the software. The dashboard image is the same for the hosted
// service and for a self-host, so a literal key or DSN in the bundle would make
// every self-hosted install report its users' errors, URLs and IPs to somebody
// else's project. Both come from the container-injected runtime config instead.
//
// Each SDK loads as its own chunk, and only when it is configured: an install
// with neither fetches nothing and contacts nobody. That would normally cost
// the errors thrown while a chunk is still in flight, which are exactly the
// ones a broken deploy produces, so this file installs its own two-line
// `error` / `unhandledrejection` listeners first and replays what they caught
// into each backend as it arrives. Ours come off once every configured backend
// has installed its own, so nothing is reported twice.
//
// Two things make a reported error answerable rather than just countable, and
// both are fanned out from here:
//
//   - `setErrorIdentity` names the workspace and user an exception belongs to,
//     so "this customer says the campaign page breaks" is a search.
//   - `noteStep` leaves a trail (route changes, failed requests with their
//     request id) that the next exception carries, so the issue shows what led
//     to it without recording anybody's screen.
import { POSTHOG_ERROR_TRACKING, POSTHOG_KEY, SENTRY_DSN, SENTRY_ENVIRONMENT, SENTRY_RELEASE } from "./information";
import { loadPostHog, notePostHogStep, setPostHogIdentity } from "./posthog";

export type Identity = { organizationId?: string | null; userId?: string | null } | null;
export type StepProperties = Record<string, string | number | boolean>;

type Backend = {
    capture: (error: unknown) => void;
    identify: (identity: Identity) => void;
    step: (message: string, properties?: StepProperties) => void;
};

const backends: Backend[] = [];

// EARLY_LIMIT bounds the pre-load buffers: a render loop that throws every
// frame must not grow them without end.
const EARLY_LIMIT = 20;
let early: unknown[] = [];
let earlySteps: Array<{ message: string; properties?: StepProperties }> = [];

// identity is remembered rather than forwarded once, because a backend that
// finishes loading after sign-in still has to learn who is signed in.
let identity: Identity = null;

// awaiting counts the backends still loading. At zero the buffers are done.
let awaiting = 0;
let removeEarlyHandlers: (() => void) | null = null;

// initErrorReporting is called once, before the app renders.
export function initErrorReporting(): void {
    const postHog = Boolean(POSTHOG_KEY) && POSTHOG_ERROR_TRACKING;
    const sentry = Boolean(SENTRY_DSN);
    if (!postHog && !sentry) return;

    installEarlyHandlers();

    if (postHog) {
        awaiting++;
        void loadPostHog().then((client) =>
            settle(client
                ? {
                      capture: (error) => void client.captureException(error),
                      identify: setPostHogIdentity,
                      step: notePostHogStep,
                  }
                : null),
        );
    }

    if (sentry) {
        awaiting++;
        void import("@sentry/react")
            .then((Sentry) => {
                Sentry.init({
                    dsn: SENTRY_DSN,
                    sendDefaultPii: true,
                    environment: SENTRY_ENVIRONMENT,
                    // Empty is omitted rather than sent: an event tagged with
                    // the empty release matches no uploaded source map and
                    // reads as a real release.
                    release: SENTRY_RELEASE || undefined,
                });
                settle({
                    capture: (error) => void Sentry.captureException(error),
                    identify: (next) =>
                        Sentry.setUser(next?.userId
                            ? { id: next.userId, organization_id: next.organizationId ?? undefined }
                            : null),
                    step: (message, properties) =>
                        Sentry.addBreadcrumb({ category: "app", message, data: properties, level: "info" }),
                });
            })
            .catch(() => settle(null));
    }
}

// captureException reports an error the app handled itself. A no-op when no
// backend is configured.
export function captureException(error: unknown): void {
    // Remembered as well as reported while a backend is still loading, so the
    // one that has not arrived yet gets it on replay. Only the newly settled
    // backend replays, so nothing is reported twice.
    if (awaiting > 0) remember(error);
    for (const backend of backends) backend.capture(error);
}

// setErrorIdentity names the workspace and user later exceptions belong to.
// Pass null on sign-out. Analytics never sees this: it is attached to exception
// events only, and no profile is created for it.
export function setErrorIdentity(next: Identity): void {
    identity = next;
    for (const backend of backends) backend.identify(next);
}

// noteStep adds one step to the trail the next exception carries. Keep the
// message bounded, a route pattern rather than a record id, and keep it and the
// properties free of a contact's name, an email address or a subject line.
export function noteStep(message: string, properties?: StepProperties): void {
    if (awaiting > 0 && earlySteps.length < EARLY_LIMIT) earlySteps.push({ message, properties });
    for (const backend of backends) backend.step(message, properties);
}

function settle(backend: Backend | null): void {
    if (backend) {
        backends.push(backend);
        if (identity) backend.identify(identity);
        for (const step of earlySteps) backend.step(step.message, step.properties);
        for (const error of early) backend.capture(error);
    }

    awaiting--;
    if (awaiting > 0) return;

    // Every configured backend has its own global handlers installed by now, so
    // keeping ours would report the next unhandled error twice.
    removeEarlyHandlers?.();
    removeEarlyHandlers = null;
    early = [];
    earlySteps = [];
}

function installEarlyHandlers(): void {
    if (removeEarlyHandlers || typeof window === "undefined") return;

    const onError = (event: ErrorEvent) => remember(event.error ?? event.message);
    const onRejection = (event: PromiseRejectionEvent) => remember(event.reason);
    window.addEventListener("error", onError);
    window.addEventListener("unhandledrejection", onRejection);

    removeEarlyHandlers = () => {
        window.removeEventListener("error", onError);
        window.removeEventListener("unhandledrejection", onRejection);
    };
}

function remember(error: unknown): void {
    if (early.length >= EARLY_LIMIT) return;
    early.push(error);
}
