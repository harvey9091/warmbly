// Issue #396: opening an email sent the conversation list back to the top.
//
// The cause was structural, not visual: the shell keyed its route boundary on
// the pathname, and the unibox puts the open thread IN the pathname, so every
// click tore the page down and built a new one. This mounts the real shell
// (RootAppLayout -> AppShell -> RouteBoundary -> Suspense -> Outlet) around the
// real unibox route and pins both halves of the fix: the list survives the
// click, and a remembered offset is put back whenever something else does zero
// it (the mobile pane, or leaving the inbox and coming back).

import React from "react";
import { describe, it, expect, vi, beforeAll, beforeEach } from "vitest";
import { render, screen, act, fireEvent } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// jsdom has no layout: scrollTop is a hard 0 and the height properties do not
// exist. Back them with real values so a scroll offset is something the test
// can set, read, and watch survive.
let tops = new WeakMap<Element, number>();
beforeAll(() => {
    Object.defineProperty(Element.prototype, "scrollTop", {
        configurable: true,
        get(this: Element) {
            return tops.get(this) ?? 0;
        },
        set(this: Element, value: number) {
            tops.set(this, value);
        },
    });
    Object.defineProperty(Element.prototype, "clientHeight", {
        configurable: true,
        get: () => 400,
    });
    Object.defineProperty(Element.prototype, "scrollHeight", {
        configurable: true,
        get: () => 4000,
    });
    (Element.prototype as unknown as { scrollTo: () => void }).scrollTo = () => {};
    (Element.prototype as unknown as { scrollIntoView: () => void }).scrollIntoView = () => {};
});

vi.mock("@/lib/api/client/Request", () => ({
    default: (cfg: { url?: string }) => Promise.resolve(route(String(cfg?.url ?? ""))),
}));
vi.mock("@/lib/helper/getToken", () => ({
    default: () => ({
        access_token: "a",
        refresh_token: "r",
        access_token_expires_at: new Date(Date.now() + 3600e3).toISOString(),
        refresh_token_expires_at: new Date(Date.now() + 3600e3).toISOString(),
    }),
}));
vi.mock("@/hooks/SocketProvider", () => ({
    default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("@/hooks/context/socket", async (orig) => {
    const actual = (await orig()) as Record<string, unknown>;
    return {
        ...actual,
        useSocket: () => ({
            isConnected: false,
            subscribeToChannel: () => () => {},
            pushToChannel: () => {},
            socket: null,
            status: "closed",
        }),
        useChannel: () => ({ state: "closed", push: () => {}, channel: null }),
        useChannelEvent: () => {},
        useChannelSubscription: () => {},
    };
});

const EMPTY_LIST = { data: [], pagination: { total: 0, next_cursor: null, has_more: false } };

const ROWS = Array.from({ length: 12 }, (_, i) => ({
    id: `msg-${i}`,
    email_id: "mbox-1",
    thread_id: `thread-${i}`,
    from_addr: [`Sender ${i} <s${i}@example.com>`],
    to_addr: ["me@warmbly.com"],
    subject: `Subject ${i}`,
    snippet: `Snippet ${i}`,
    internal_date: new Date(Date.now() - i * 3600e3).toISOString(),
    seen: true,
    message_count: 1,
    has_unread: false,
    labels: [],
}));

function route(url: string): unknown {
    if (url === "/auth/me" || url === "/me") {
        return {
            id: "u1", email: "d@w.com", first_name: "D", last_name: "W",
            onboarding_completed_at: new Date().toISOString(),
            tags: [], categories: [], folders: [], roles: [],
        };
    }
    // billing_enabled:false unlocks every feature gate, so the inbox renders
    // for real instead of behind the upgrade overlay.
    if (url.startsWith("/auth/config")) {
        return {
            captcha: false, password_login: true, login_code: "off",
            registration: "invite_only", invites_required: true,
            email_verification: false, mail_delivers: false, passkeys: false,
            providers: [], self_hosted: true, billing_enabled: false,
            setup_required: false, docs_url: "",
        };
    }
    if (url.startsWith("/organization")) return [{ id: "org-1", name: "Org", slug: "org", role: "owner" }];
    if (url.startsWith("/subscription/credits")) {
        return { monthly_balance: 100, monthly_allowance: 100, purchased_balance: 0, spent_today: 0, spent_week: 0, spent_month: 0 };
    }
    if (url.startsWith("/subscription")) return { plan: { name: "Pro" }, status: "active" };
    if (url.startsWith("/unibox/overview")) {
        return { total: ROWS.length, unread: 0, awaiting_reply: 0, snoozed: 0, today: 0, week: 0, mailboxes: [], tags: [], categories: [], folders: [] };
    }
    if (url.startsWith("/unibox/count")) return { count: 0 };
    if (url.startsWith("/unibox/thread")) {
        return { data: [{ ...ROWS[0], seen: true }], pagination: { has_more: false, next_cursor: null } };
    }
    if (url === "/unibox" || url.startsWith("/unibox?")) {
        return { data: ROWS, pagination: { has_more: false, next_cursor: null } };
    }
    if (url.startsWith("/analytics")) return { summary: {}, steps: [], data: [] };
    if (url.startsWith("/advisor")) return { findings: [], data: [], total: 0 };
    return EMPTY_LIST;
}

const RootAppLayout = (await import("../layout")).default;
const UniboxPage = (await import("./page")).default;

function Elsewhere() {
    return <div>Somewhere else</div>;
}

function mount(initial = "/app/unibox/all") {
    const router = createMemoryRouter(
        [
            {
                path: "/app",
                element: <RootAppLayout />,
                children: [
                    {
                        path: "unibox/:scope?/:threadId?",
                        element: <UniboxPage />,
                        handle: { stableParams: ["scope", "threadId"] },
                    },
                    { path: "analytics", element: <Elsewhere /> },
                ],
            },
        ],
        { initialEntries: [initial] },
    );
    render(
        <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
            <RouterProvider router={router} />
        </QueryClientProvider>,
    );
    return router;
}

async function settle() {
    await act(async () => {
        await new Promise((r) => setTimeout(r, 300));
    });
}

function scroller(): HTMLElement {
    const row = document.querySelector("[data-thread-id]");
    const el = row?.closest<HTMLElement>(".overflow-y-auto");
    if (!el) throw new Error("conversation list scroll container not found");
    return el;
}

async function scrollTo(top: number) {
    const el = scroller();
    el.scrollTop = top;
    await act(async () => {
        fireEvent.scroll(el);
    });
}

describe("unibox scroll position", () => {
    // The fake offsets are per element and the hook's own memory is per list
    // identity, so tests reset the first and take a scope of their own for the
    // second. Otherwise a passing assertion could be the previous test's.
    beforeEach(() => {
        tops = new WeakMap<Element, number>();
    });

    it("keeps the list where it was when a thread is opened", async () => {
        const router = mount("/app/unibox/all");
        await settle();
        expect(screen.queryByText("Subject 4")).toBeTruthy();

        const before = scroller();
        await scrollTo(1200);

        await act(async () => {
            fireEvent.click(screen.getByText("Subject 4").closest("button")!);
        });
        await settle();

        expect(router.state.location.pathname).toBe("/app/unibox/all/thread-4");
        // Same DOM node: the page was never torn down, which is the whole fix.
        expect(scroller()).toBe(before);
        expect(scroller().scrollTop).toBe(1200);
    });

    it("keeps what the user typed into the list search when a thread is opened", async () => {
        mount("/app/unibox/unread");
        await settle();

        const search = screen.getByPlaceholderText(/^Search unread/i) as HTMLInputElement;
        await act(async () => {
            fireEvent.change(search, { target: { value: "invoice" } });
        });
        await settle();

        await act(async () => {
            fireEvent.click(screen.getByText("Subject 2").closest("button")!);
        });
        await settle();

        expect(
            (screen.getByPlaceholderText(/^Search unread/i) as HTMLInputElement).value,
        ).toBe("invoice");
    });

    it("puts a remembered offset back after the pane is hidden and shown again", async () => {
        // Below `md` the list is display:none while a thread is open, and the
        // browser zeroes a hidden scroller without firing a scroll event. Same
        // thing here: move the offset behind the component's back, then render.
        mount("/app/unibox/today");
        await settle();
        await scrollTo(700);

        await act(async () => {
            fireEvent.click(screen.getByText("Subject 3").closest("button")!);
        });
        await settle();
        scroller().scrollTop = 0;

        // The thread pane's back link, the mobile way back to the list.
        const back = screen
            .getAllByRole("button", { name: "Inbox" })
            .find((b) => b.className.includes("md:hidden"))!;
        await act(async () => {
            fireEvent.click(back);
        });
        await settle();

        expect(scroller().scrollTop).toBe(700);
    });

    it("puts a remembered offset back when the inbox is re-entered", async () => {
        const router = mount("/app/unibox/week");
        await settle();
        await scrollTo(900);

        await act(async () => {
            await router.navigate("/app/analytics");
        });
        await settle();
        expect(screen.queryByText("Somewhere else")).toBeTruthy();

        await act(async () => {
            await router.navigate("/app/unibox/week");
        });
        await settle();

        expect(scroller().scrollTop).toBe(900);
    });
});
