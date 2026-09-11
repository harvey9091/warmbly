// Browser error reporting for the operator panel.
//
// Same rule as the dashboard: PostHog is the default backend and Sentry is
// still supported, both come from the container-injected runtime config, and an
// install that configures neither reports nothing anywhere. The admin image
// ships to self-hosters too, so a literal key or DSN here would make every
// self-hosted panel report its operator's errors and URLs to somebody else.
//
// Each SDK loads as its own chunk and only when it is configured, so an install
// with neither fetches nothing. The two listeners below cover the window while
// a chunk is in flight, which is exactly when a broken deploy throws, and come
// off once every configured backend has installed its own.
//
// Nothing about an operator's session is measured here: no pageviews, no
// autocapture, no person profile. Exceptions only, and what they carry is the
// operator whose request it was and the trail that led there, so an issue can
// be answered rather than only counted.
import { POSTHOG_ERROR_TRACKING, POSTHOG_HOST, POSTHOG_KEY, POSTHOG_UI_HOST, SENTRY_DSN, SENTRY_ENVIRONMENT, SENTRY_RELEASE } from "./env";

export type Identity = { userId?: string | null } | null;
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
        void import("posthog-js")
            .then(({ posthog }) => {
                posthog.init(POSTHOG_KEY, {
                    api_host: POSTHOG_HOST,
                    ui_host: POSTHOG_UI_HOST,
                    cookieless_mode: "always",
                    person_profiles: "never",
                    autocapture: false,
                    capture_pageview: false,
                    disable_session_recording: true,
                    respect_dnt: true,
                    // Console errors stay off: they are mostly third-party
                    // noise and they would carry logged values.
                    capture_exceptions: {
                        capture_unhandled_errors: true,
                        capture_unhandled_rejections: true,
                        capture_console_errors: false,
                    },
                });
                // Registered rather than passed per call so an autocaptured
                // error carries them too. Named so panel errors are separable
                // from the dashboard's in a shared project, the same way the Go
                // services set a service property.
                posthog.register(SENTRY_RELEASE
                    ? { service: "admin", environment: SENTRY_ENVIRONMENT, release: SENTRY_RELEASE }
                    : { service: "admin", environment: SENTRY_ENVIRONMENT });
                settle({
                    capture: (error) => void posthog.captureException(error),
                    // A registered property, not an identify: the panel builds
                    // no person profile and stores nothing in the browser. The
                    // panel sends nothing but exceptions, so this reaches
                    // nothing else.
                    identify: (next) =>
                        next?.userId
                            ? posthog.register({ user_id: next.userId })
                            : posthog.unregister("user_id"),
                    step: (message, properties) => posthog.addExceptionStep(message, properties),
                });
            })
            .catch(() => settle(null));
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
                    identify: (next) => Sentry.setUser(next?.userId ? { id: next.userId } : null),
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

// setErrorIdentity names the operator later exceptions belong to. Pass null on
// sign-out.
export function setErrorIdentity(next: Identity): void {
    identity = next;
    for (const backend of backends) backend.identify(next);
}

// noteStep adds one step to the trail the next exception carries. Keep the
// message short and free of anything about a customer's data.
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
