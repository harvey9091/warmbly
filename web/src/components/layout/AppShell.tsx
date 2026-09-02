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

import { Suspense, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import SubscriptionGate from "./SubscriptionGate";
import { SkyChrome } from "./SkyChrome";
import { AppHeader } from "./AppHeader";
import { AppNav } from "./AppNav";
import PendingDeletionBar from "./PendingDeletionBar";
import SendingRestrictedBar from "./SendingRestrictedBar";
import { RouteBoundary } from "./ErrorBoundary";
import { ShortcutsModal } from "@/components/shared/ShortcutsModal";
import { CommandPalette } from "@/components/shared/CommandPalette";
import { useKeyboardShortcuts } from "@/hooks/useKeyboardShortcuts";
import { GlobalCursorsProvider } from "@/components/app/presence/GlobalCursors";
import AgentPanel from "@/components/app/agent/AgentPanel";
import { BackgroundLayer } from "@/components/appearance/BackgroundLayer";
import { useAppStore } from "@/stores";

export function AppShell() {
    useKeyboardShortcuts();

    const glassmorphismEnabled = useAppStore((state) => state.glassmorphismEnabled)
    const glassOpacity = useAppStore((state) => state.glassOpacity)
    const glassBlur = useAppStore((state) => state.glassBlur)

    const [navOpen, setNavOpen] = useState(false);
    const { pathname } = useLocation();
    useEffect(() => setNavOpen(false), [pathname]);

    const scrollRef = useRef<HTMLDivElement>(null);

    useLayoutEffect(() => {
        scrollRef.current?.scrollTo({ top: 0, left: 0 });
    }, [pathname]);

    useEffect(() => {
        const root = document.documentElement
        root.style.setProperty('--app-glass-opacity', String(glassmorphismEnabled ? glassOpacity / 100 : 0.15))
        root.style.setProperty('--app-glass-blur', `${glassBlur}px`)
    }, [glassmorphismEnabled, glassOpacity, glassBlur])

    return (
        <div className={`fixed inset-0 flex flex-col ${glassmorphismEnabled ? "glassmorphism-enabled" : ""}`}>
            <BackgroundLayer />

            <SkyChrome />

            <div className="relative z-10 flex flex-col h-full">
                <SendingRestrictedBar />
                <PendingDeletionBar />

                <AppHeader onMenu={() => setNavOpen(true)} />

                <div className="flex-1 flex min-h-0">
                    <AppNav open={navOpen} onClose={() => setNavOpen(false)} />

                    <main className="app-shell-content flex-1 min-w-0 bg-background overflow-hidden border-t border-slate-200/70 md:rounded-tl-2xl md:border-l">
                        <GlobalCursorsProvider scrollRef={scrollRef}>
                            <div ref={scrollRef} className="h-full overflow-auto">
                                <RouteBoundary>
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

            <ShortcutsModal />
            <CommandPalette />
            <AgentPanel />
        </div>
    );
}

// RouteFallback is what a suspending page shows while its data loads. Same
// hairline chrome as the pages themselves so the panel never goes blank.
function RouteFallback() {
    return (
        <div className="px-5 pt-5 space-y-4" role="status" aria-label="Loading">
            <div className="h-6 w-56 bg-slate-100 rounded-md animate-pulse" />
            <div className="h-3 w-40 bg-slate-100 rounded animate-pulse" />
            <div className="h-56 bg-slate-100 rounded-md animate-pulse" />
        </div>
    );
}
