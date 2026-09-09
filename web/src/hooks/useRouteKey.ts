// Identity of the page the router is currently showing.
//
// The shell uses it for the two things that must happen when you move to a
// different page and must NOT happen when you only move around inside one:
// remounting the route's Suspense boundary, and putting the content panel back
// at the top.
//
// Default identity is the full pathname, so every URL change is a new page.
// A route whose path also carries in-page state opts those params out:
//
//   { path: "unibox/:scope?/:threadId?", element: <UniboxPage />,
//     handle: { stableParams: ["scope", "threadId"] } }
//
// Changing only a stable param keeps the page mounted, so its lists keep their
// scroll offset and its inputs keep their text (issue #396).

import { useLocation, useMatches } from "react-router-dom";

export interface RouteHandle {
    /** Route params that address state inside the page, not a different page. */
    stableParams?: string[];
}

export function useRouteKey(): string {
    const { pathname } = useLocation();
    const matches = useMatches();
    const deepest = matches[matches.length - 1];
    const stable = (deepest?.handle as RouteHandle | undefined)?.stableParams;
    if (!deepest || !stable || stable.length === 0) return pathname;

    const params = (deepest.params ?? {}) as Record<string, string | undefined>;
    const rest = Object.keys(params)
        .filter((name) => !stable.includes(name))
        .sort()
        .map((name) => `${name}=${params[name] ?? ""}`)
        .join("&");
    return `${deepest.id}?${rest}`;
}
