// Terminal states, word-for-word with the static pages the Go service serves
// for the same conditions at the shell level.

export function NotFound() {
    return (
        <div className="plain">
            <h1>This form is no longer available</h1>
            <p>It may have been unpublished or removed.</p>
        </div>
    );
}

export function Unavailable() {
    return (
        <div className="plain">
            <h1>This form is temporarily unavailable</h1>
            <p>Please try again in a moment.</p>
        </div>
    );
}

// StalePage: the tab outlived its render token (12h); only a reload mints a
// new one.
export function StalePage() {
    return (
        <div className="plain">
            <h1>This page went stale</h1>
            <p>It has been open for a while. Refresh to continue.</p>
            <button type="button" onClick={() => window.location.reload()}>
                Refresh
            </button>
        </div>
    );
}
