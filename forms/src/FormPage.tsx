// The /f/$publicId route: fetch the form (with any personalized ?t= ticket),
// paint its theme, run the embed plumbing and funnel beacons, render the
// chosen layout. The Go service already 404s unknown ids at the shell, so the
// in-app not-found path only fires when a form unpublishes mid-visit.

import type React from "react";
import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";

import { fetchForm, FormNotFoundError, personalToken, StalePageError } from "./api";
import { applyDesign, resolveDesign, splitPages } from "./design";
import { isEmbedded, startResizeReporting } from "./embed";
import { makeTracker } from "./events";
import { FormRenderer } from "./FormRenderer";
import { NotFound, StalePage, Unavailable } from "./NotFound";

export function FormPage() {
    const { publicId } = useParams({ from: "/f/$publicId" });
    const linkToken = personalToken();
    const query = useQuery({
        queryKey: ["form", publicId, linkToken],
        queryFn: () => fetchForm(publicId, linkToken),
        staleTime: Infinity,
        retry: (count, err) =>
            !(err instanceof FormNotFoundError) && !(err instanceof StalePageError) && count < 2,
    });

    const form = query.data;
    const design = useMemo(() => (form ? resolveDesign(form.design) : null), [form]);
    const tracker = useMemo(
        () => (form ? makeTracker(form.public_id, splitPages(form.fields).length) : null),
        [form],
    );

    useEffect(() => {
        if (!form || !design || !tracker) return;
        document.title = form.name;
        applyDesign(design);
        if (isEmbedded()) document.body.classList.add("embed");
        tracker.view();
        tracker.page(0);
        return startResizeReporting(form.public_id);
    }, [form, design, tracker]);

    if (query.isPending) return null;
    if (query.isError) {
        if (query.error instanceof StalePageError) return <StalePage />;
        return query.error instanceof FormNotFoundError ? <NotFound /> : <Unavailable />;
    }
    if (!form || !design || !tracker) return null;

    const logo = form.logo_url ? <img className="wf-logo" src={form.logo_url} alt="" /> : null;
    // The header only claims the logo when it is set to show one; otherwise the
    // logo keeps its own placement and the header carries just the title.
    const headerLogo = design.headerEnabled && design.headerShowLogo ? logo : null;
    const bodyLogo = headerLogo ? null : logo;
    // A header with neither a logo nor a title is an empty strip, so the live
    // page skips it; the builder shows a placeholder in its place instead.
    const headerEl =
        design.headerEnabled && (headerLogo || design.headerTitle) ? (
            <header
                className={[
                    "wf-header",
                    `h-${design.headerAlign}`,
                    design.headerPlacement === "inline" ? "wf-header--inline" : "",
                    design.headerSticky ? "is-sticky" : "",
                ]
                    .filter(Boolean)
                    .join(" ")}
            >
                {headerLogo}
                {design.headerTitle && <span className="wf-header-title">{design.headerTitle}</span>}
            </header>
        ) : null;

    return (
        <div
            className={`wf layout-${design.layout} mode-${design.mode} align-${design.align}`}
            // The background image is a form asset like the logo, so it is set
            // here rather than carried in the design variables.
            style={
                form.background_url
                    ? ({ "--wf-bg-image": `url(${JSON.stringify(form.background_url)})` } as React.CSSProperties)
                    : undefined
            }
        >
            {design.headerPlacement === "page" && headerEl}
            <div className="wf-body">
                {design.layout === "split" && (
                    <aside
                        className="wf-cover"
                        style={form.cover_url ? { backgroundImage: `url(${JSON.stringify(form.cover_url)})` } : undefined}
                    >
                        {bodyLogo}
                        {design.coverTitle && <h2 className="wf-cover-title">{design.coverTitle}</h2>}
                        {design.coverSubtitle && <p className="wf-cover-sub">{design.coverSubtitle}</p>}
                    </aside>
                )}
                <main className="wf-main">
                    {design.layout !== "split" && design.logoOnPage && bodyLogo}
                    <div className={design.layout === "card" ? "card" : "wf-bare"}>
                        {design.headerPlacement === "inline" && headerEl}
                        {design.layout !== "split" && !design.logoOnPage && bodyLogo}
                        <FormRenderer form={form} design={design} tracker={tracker} />
                    </div>
                    <div className="brand">
                        <a href="https://warmbly.com" target="_blank" rel="noopener noreferrer">
                            Powered by Warmbly
                        </a>
                    </div>
                </main>
            </div>
        </div>
    );
}
