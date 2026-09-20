// Triage from the conversation list: the row's own actions, and the selection
// bar behind the checkboxes.
//
// Archive used to look like it had done nothing. The row left the list on the
// optimistic removal and the refetch that followed put it straight back,
// because every scope but Inbox listed archived mail too. The server half of
// that is pinned in Go; this pins the client half: the scopes that must ask
// for filed conversations back, and the ones that must not.

import React from "react";
import { describe, it, expect, vi, beforeAll, beforeEach } from "vitest";
import { screen, act, fireEvent, waitFor, within } from "@testing-library/react";
import {
    installLayoutShims,
    mount,
    resetScrollTops,
    ROWS,
    setViewportWidth,
    settle,
    SUITE,
} from "./uniboxHarness";

type Call = { method?: string; url?: string; data?: Record<string, unknown> };

const calls = vi.hoisted((): Call[] => []);

beforeAll(() => {
    installLayoutShims();
    setViewportWidth(1512);
});

vi.mock("@/lib/api/client/Request", () => ({
    default: async (cfg: Call) => {
        calls.push({ method: cfg.method, url: cfg.url, data: cfg.data });
        const { route } = await import("./uniboxHarness");
        return route(String(cfg?.url ?? ""));
    },
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

const listUrls = () =>
    calls
        .filter((c) => c.url === "/unibox" || c.url?.startsWith("/unibox?"))
        .map((c) => String(c.url));

const patches = (path: string) =>
    calls.filter((c) => c.method === "PATCH" && c.url === path);

// The rows carry Archive buttons of their own, so the bar's controls are
// always reached through the bar.
const selectionBar = () =>
    screen.getByRole("toolbar", { name: "Selection actions" });

describe("unibox triage", SUITE, () => {
    beforeEach(() => {
        calls.length = 0;
        resetScrollTops();
        setViewportWidth(1512);
    });

    // All mail is the one view a filed conversation stays in.
    it("asks for filed conversations in All mail", async () => {
        await mount("/app/unibox/all");
        await settle();
        expect(listUrls().some((u) => u.includes("include_archived=true"))).toBe(true);
    });

    // Every other scope has to leave them out, or Archive removes a row that
    // the next read returns.
    it("leaves filed conversations out of a working view", async () => {
        await mount("/app/unibox/awaiting");
        await settle();
        const awaiting = listUrls();
        expect(awaiting.some((u) => u.includes("awaiting_reply=true"))).toBe(true);
        expect(awaiting.some((u) => u.includes("include_archived"))).toBe(false);
    });

    // The row knows its conversation and not the message ids inside it, so it
    // files by thread; filing part of one leaves the row where it was.
    it("archives from the row, addressing the whole conversation", async () => {
        await mount("/app/unibox/awaiting");
        await settle();

        const row = screen.getByText(ROWS[0].subject).closest('[role="button"]')!;
        await act(async () => {
            fireEvent.click(row.querySelector('[aria-label="Archive"]')!);
        });
        await settle();

        const [filed] = patches("/unibox/folder");
        expect(filed).toBeTruthy();
        expect(filed.data).toMatchObject({
            folder: "archive",
            thread_ids: [ROWS[0].thread_id],
        });
    });

    // Opening a conversation is not what the row's own actions are for.
    it("does not open the conversation when a row action is used", async () => {
        const router = await mount("/app/unibox/awaiting");
        await settle();

        const row = screen.getByText(ROWS[1].subject).closest('[role="button"]')!;
        await act(async () => {
            fireEvent.click(row.querySelector('[aria-label="Conversation actions"]')!);
        });
        await settle();

        expect(router.state.location.pathname).toBe("/app/unibox/awaiting");
        expect(screen.getByText("Mark as unread")).toBeTruthy();
    });

    it("marks a whole selection read from the selection bar", async () => {
        await mount("/app/unibox/awaiting");
        await settle();

        for (const i of [0, 1]) {
            const row = screen.getByText(ROWS[i].subject).closest('[role="button"]')!;
            await act(async () => {
                fireEvent.click(row.querySelector('input[type="checkbox"]')!);
            });
        }
        await settle();

        await waitFor(() => expect(screen.getByText("2 selected")).toBeTruthy());

        calls.length = 0;
        await act(async () => {
            fireEvent.click(within(selectionBar()).getByTitle("Mark read"));
        });
        await settle();

        const [seen] = patches("/unibox/seen");
        expect(seen).toBeTruthy();
        expect(seen.data).toMatchObject({
            seen: true,
            thread_ids: [ROWS[0].thread_id, ROWS[1].thread_id],
        });
        // The rows the bar acted on are no longer ticked: a count that outlives
        // what it applied to is a lie.
        await waitFor(() => expect(screen.queryByText("2 selected")).toBeNull());
    });

    it("archives the whole selection in one call", async () => {
        await mount("/app/unibox/awaiting");
        await settle();

        for (const i of [0, 1, 2]) {
            const row = screen.getByText(ROWS[i].subject).closest('[role="button"]')!;
            await act(async () => {
                fireEvent.click(row.querySelector('input[type="checkbox"]')!);
            });
        }
        await settle();

        calls.length = 0;
        await act(async () => {
            fireEvent.click(within(selectionBar()).getByTitle("Archive"));
        });
        await settle();

        const filed = patches("/unibox/folder");
        expect(filed).toHaveLength(1);
        expect(filed[0].data).toMatchObject({
            folder: "archive",
            thread_ids: [ROWS[0].thread_id, ROWS[1].thread_id, ROWS[2].thread_id],
        });
    });

    // Shift picks the run between the two, the way a file list does.
    it("extends a selection with shift", async () => {
        await mount("/app/unibox/awaiting");
        await settle();

        const box = (i: number) =>
            screen
                .getByText(ROWS[i].subject)
                .closest('[role="button"]')!
                .querySelector('input[type="checkbox"]')!;

        await act(async () => {
            fireEvent.click(box(0));
        });
        await act(async () => {
            fireEvent.click(box(3), { shiftKey: true });
        });
        await settle();

        await waitFor(() => expect(screen.getByText("4 selected")).toBeTruthy());
    });
});
