// The new app shell.
//
// Layout:
//   ┌──────────────────────────────────────────────────────┐
//   │  [logo]  >  [org]  >  [section]               [⌘K ●] │  AppHeader
//   ├──────────┬───────────────────────────────────────────┤
//   │          │ ╭─── content ──────────────────────────╮  │
//   │  AppNav  │ │                                      │  │
//   │          │ │                                      │  │
//   │          │ │                                      │  │
//   └──────────┴───────────────────────────────────────────┘
//
// The header + sidebar share one sky-coloured chrome layer (SkyChrome).
// The content panel sits in the bottom-right with a rounded top-left
// where it meets the chrome's inner corner. Reads as one continuous
// frame around a clean work surface.

import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { Outlet, useLocation } from "@tanstack/react-router";
import SubscriptionGate from "./SubscriptionGate";
import { GlassBackground } from "./GlassBackground";
import { AppHeader } from "./AppHeader";
import { AppNav } from "./AppNav";
import PendingDeletionBar from "./PendingDeletionBar";
import SendingRestrictedBar from "./SendingRestrictedBar";
import TrialBar from "./TrialBar";
import { useClientBrand } from "@/hooks/useClientBrand";
import { useBrandTheme } from "@/hooks/useBrandTheme";
import BalanceBanner from "@/components/app/billing/BalanceBanner";
import { RouteBoundary } from "./ErrorBoundary";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { GlobalCursorsProvider } from "@/components/app/presence/GlobalCursors";
import { useRouteKey } from "@/hooks/useRouteKey";
import { useScrollMemory, type ScrollStore } from "@/hooks/useScrollMemory";
import { RouteFallback } from "./RouteStates";
import { rememberView } from "@/lib/agencyView";
import { useAppStore } from "@/stores";


// Overlays closed on first paint, so the frame does not wait on their code.
const ShortcutsModal = lazy(() => import("@/components/shared/ShortcutsModal").then((m) => ({ default: m.ShortcutsModal })));
const CommandPalette = lazy(() => import("@/components/shared/CommandPalette").then((m) => ({ default: m.CommandPalette })));
const AgentPanel = lazy(() => import("@/components/app/agent/AgentPanel"));
const ProductTourHost = lazy(() => import("@/components/app/tour/ProductTourHost"));

// Per history entry, kept apart so ordinary browsing never evicts a list's remembered offset.
const entryOffsets: ScrollStore = new Map();

export function AppShell() {
    useKeyboardShortcuts();

    // Mobile nav drawer. On >=md the sidebar is a static column and this is
    // ignored; below md it's an off-canvas drawer toggled from the header.
    const [navOpen, setNavOpen] = useState(false);
    // A client member of a branded workspace is not shown Warmbly's trial or balance prompts.
    const clientBrand = useClientBrand();
    // The same members see the dashboard in that brand's colour, font and corners.
    useBrandTheme(clientBrand);
    const { pathname } = useLocation();
    // Close the drawer whenever the route changes (tapping a nav link).
    useEffect(() => setNavOpen(false), [pathname]);
    useEffect(() => rememberView("workspace"), []);

    // Apply glassmorphism theme globally
    const glassBackground = useAppStore((state) => state.glassBackground);
    const glassBlurLevel = useAppStore((state) => state.glassBlurLevel);
    const glassTransparency = useAppStore((state) => state.glassTransparency);
    useEffect(() => {
        const root = document.documentElement;

        // Toggle glassmorphism class
        if (glassBackground) {
            root.classList.add('glassmorphism');
        } else {
            root.classList.remove('glassmorphism');
        }

        // Set backdrop filter
        root.style.setProperty('--glass-backdrop-filter', `blur(${glassBlurLevel}px)`);
        root.style.setProperty('--glass-backdrop-filter-light', `blur(${glassBlurLevel}px)`);
        root.style.setProperty('--glass-backdrop-filter-dark', `blur(${glassBlurLevel + 4}px)`);

        // Set transparency (0–100 → 0.0–1.0)
        const alpha = glassTransparency / 100;
        root.style.setProperty('--glass-background', `rgba(255, 255, 255, ${alpha})`);
        root.style.setProperty('--glass-card', `rgba(255, 255, 255, ${alpha * 0.9})`);
        root.style.setProperty('--glass-popover', `rgba(255, 255, 255, ${alpha * 0.85})`);
        root.style.setProperty('--glass-border', `rgba(255, 255, 255, ${alpha * 0.2})`);
        root.style.setProperty('--glass-input', `rgba(255, 255, 255, ${alpha * 0.7})`);
        root.style.setProperty('--glass-ring', `rgba(255, 255, 255, ${alpha * 0.4})`);
        root.style.setProperty('--glass-shadow', `rgba(0, 0, 0, ${0.1 * alpha})`);

        // Set background classes
        root.classList.remove('glass-bg-1', 'glass-bg-2');
        if (glassBackground === 'bg1') {
            root.classList.add('glass-bg-1');
        } else if (glassBackground === 'bg2') {
            root.classList.add('glass-bg-2');
        }
    }, [glassBackground, glassBlurLevel, glassTransparency]);

    // The page content's scroll container, anchor for the global cursor layer.
    const scrollRef = useRef<HTMLDivElement>(null);

    // A new page starts at the top and Back lands where that entry was left; in-page URL state keeps its place.
    const routeKey = useRouteKey();
    const entryKey = useLocation({ select: (l) => l.state.__TSR_key ?? l.href });
    const [scrollFor, setScrollFor] = useState({ routeKey, key: `${routeKey}@${entryKey}` });
    if (scrollFor.routeKey !== routeKey) setScrollFor({ routeKey, key: `${routeKey}@${entryKey}` });
    useScrollMemory(scrollRef, scrollFor.key, entryOffsets);

    return (
        <div className="fixed inset-0 flex flex-col">
            {/* Global background layer - covers entire viewport */}
            <GlassBackground />

            {/* Main application content layer */}
            <div className="relative z-10 flex flex-col h-full">
                {/* Sits above the header so it can't be missed. Only
                    renders when the current workspace or the user's
                    own account is scheduled for deletion. */}
                <SendingRestrictedBar />
                <PendingDeletionBar />
                {!clientBrand && <TrialBar />}
                {!clientBrand && <BalanceBanner />}

                <AppHeader onMenu={() => setNavOpen(true)} />

                <div className="flex-1 flex min-h-0">
                    <AppNav open={navOpen} onClose={() => setNavOpen(false)} />

                    {/* Content panel — pure white work surface. The inner
                        corner is softened (rounded-tl-2xl) only on >=md, where
                        the sidebar sits beside it; on mobile the panel is
                        full-bleed with just a top hairline. */}
                    <main className="wb-panel flex-1 min-w-0 glass-card border-t border-slate-200/70 md:rounded-tl-2xl md:border-l">
                        {/* Dark theme only: a soft light from the top edge, laid over every page whatever it paints. */}
                        <div aria-hidden className="pointer-events-none relative z-20 hidden h-0 dark:block">
                            <div className="wb-panel-light absolute inset-x-0 top-0 h-[420px]" />
                        </div>
                        <GlobalCursorsProvider scrollRef={scrollRef}>
                            <div ref={scrollRef} className="h-full overflow-auto">
                                <RouteBoundary>
                                    {/* Catches a suspend above the route's own
                                        boundary (SubscriptionGate). */}
                                    <Suspense fallback={<RouteFallback />}> 
                                        <SubscriptionGate>
                                            <Outlet />
                                        </SubscriptionGate>
                                    </Suspense>
                                </RouteBoundary>
                            </div>
                        </GlobalCursorsProvider>
                    </main>
                </div>
            </div>

            {/* Overlay components - should be above everything else */}
            <div className="z-30">
                <Suspense fallback={null}>
                    <ShortcutsModal />
                    <CommandPalette />
                    {/* Right-side AI assistant, persistent across routes. */}
                    <AgentPanel />
                    <ProductTourHost />
                </Suspense>
            </div>
        </div>
    );
}
