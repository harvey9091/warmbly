// Issue #187: adding a contact to a campaign left the campaign Leads tab showing
// the old, cached result set. The Leads tab is a ["contacts","list"] search scoped
// to one campaign, so a contact that has just joined the campaign has no row in it
// to patch: the query has to refetch. These tests drive the real hooks against a
// fake API and assert the list ends up holding the new lead.

import React from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { QueryClient, QueryClientProvider, type InfiniteData } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type Contact from "@/lib/api/models/app/contacts/Contact";
import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";
import type SearchContactsResult from "@/lib/api/models/app/contacts/SearchContactsResult";

const searchContacts = vi.fn();
const updateContactsBulk = vi.fn();
const updateContact = vi.fn();
const addContacts = vi.fn();

vi.mock("@/lib/api/client/app/contacts/searchContacts", () => ({
    default: (...args: unknown[]) => searchContacts(...args),
}));
vi.mock("@/lib/api/client/app/contacts/updateContactsBulk", () => ({
    default: (...args: unknown[]) => updateContactsBulk(...args),
}));
vi.mock("@/lib/api/client/app/contacts/updateContact", () => ({
    default: (...args: unknown[]) => updateContact(...args),
}));
vi.mock("@/lib/api/client/app/contacts/addContacts", () => ({
    default: (...args: unknown[]) => addContacts(...args),
}));

const useSearchContacts = (await import("./useSearchContacts")).default;
const useUpdateContactsBulk = (await import("./useUpdateContactsBulk")).default;
const useUpdateContact = (await import("./useUpdateContact")).default;
const useAddContacts = (await import("./useAddContacts")).default;

const CAMPAIGN = { id: "camp-1", name: "Agency partnerships" };
const CONTACT_ID = "contact-1";

function contact(campaigns: { id: string; name: string }[]): Contact {
    return {
        id: CONTACT_ID,
        first_name: "Carlos",
        last_name: "Diaz",
        email: "carlos.diaz@piedpiper.test",
        company: "Pied Piper",
        phone: "",
        custom_fields: {},
        subscribed: true,
        campaigns,
        categories: [],
        updated_at: new Date(),
        created_at: new Date(),
    };
}

function page(data: Contact[]): SearchContactsResult {
    return { data, pagination: { total: data.length, next_cursor: null, has_more: false } };
}

// The Leads tab's search: every contact in this one campaign.
const leadsOptions: SearchContacts = {
    query: "",
    custom_field_filters: [],
    campaign_ids: [CAMPAIGN.id],
    sort_by: "created_at",
    reverse: false,
};

function wrapper(client: QueryClient) {
    return function Wrapper({ children }: { children: React.ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    };
}

// Renders the Leads tab search, waits for the first (empty) result, then runs
// `act` and reports what the list holds once the mutation has settled.
async function leadsAfter(
    client: QueryClient,
    useMutationHook: () => { mutateAsync: (v: never) => Promise<unknown> },
    payload: unknown,
) {
    const leads = renderHook(() => useSearchContacts({ options: leadsOptions }), {
        wrapper: wrapper(client),
    });
    await waitFor(() => expect(leads.result.current.isSuccess).toBe(true));
    expect(leads.result.current.contacts).toEqual([]);

    const mutation = renderHook(() => useMutationHook(), { wrapper: wrapper(client) });
    await mutation.result.current.mutateAsync(payload as never);

    await waitFor(() => expect(leads.result.current.contacts).toHaveLength(1));
    return leads.result.current.contacts;
}

describe("adding a contact to a campaign refreshes the campaign Leads tab", () => {
    let client: QueryClient;

    beforeEach(() => {
        vi.clearAllMocks();
        client = new QueryClient({
            defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
        });
        // The lead is not in the campaign on the first read and is on every read
        // after the mutation, so a stale list is visibly different from a fresh one.
        let added = false;
        searchContacts.mockImplementation(async () => page(added ? [contact([CAMPAIGN])] : []));
        updateContactsBulk.mockImplementation(async () => {
            added = true;
            return [contact([CAMPAIGN])];
        });
        updateContact.mockImplementation(async () => {
            added = true;
            return contact([CAMPAIGN]);
        });
        addContacts.mockImplementation(async () => {
            added = true;
            return [contact([CAMPAIGN])];
        });
    });

    it("bulk edit (All contacts > Edit > Add to campaigns)", async () => {
        const contacts = await leadsAfter(client, useUpdateContactsBulk, {
            contacts: [CONTACT_ID],
            add_campaigns: [CAMPAIGN.id],
            remove_campaigns: [],
            fields: [],
        });
        expect(contacts?.[0].id).toBe(CONTACT_ID);
    });

    it("single contact drawer (Details > Campaigns)", async () => {
        const contacts = await leadsAfter(client, () => useUpdateContact(CONTACT_ID), {
            campaigns: [CAMPAIGN.id],
        });
        expect(contacts?.[0].id).toBe(CONTACT_ID);
    });

    it("new contact created into the campaign", async () => {
        const contacts = await leadsAfter(client, useAddContacts, [
            { email: "carlos.diaz@piedpiper.test", campaigns: [CAMPAIGN.id] },
        ]);
        expect(contacts?.[0].id).toBe(CONTACT_ID);
    });

    it("the bulk mutation only resolves once the lists are fresh", async () => {
        // AddFromContactsDialog closes and toasts on resolve, so the refetch has to
        // be awaited by the mutation rather than raced with the dialog closing.
        const leads = renderHook(() => useSearchContacts({ options: leadsOptions }), {
            wrapper: wrapper(client),
        });
        await waitFor(() => expect(leads.result.current.isSuccess).toBe(true));
        expect(searchContacts).toHaveBeenCalledTimes(1);

        const mutation = renderHook(() => useUpdateContactsBulk(), { wrapper: wrapper(client) });
        await mutation.result.current.mutateAsync({
            contacts: [CONTACT_ID],
            add_campaigns: [CAMPAIGN.id],
            remove_campaigns: [],
            fields: [],
        });

        // Refetched (not merely marked stale) before the mutation settled.
        expect(searchContacts).toHaveBeenCalledTimes(2);
        const cached = client.getQueriesData<InfiniteData<SearchContactsResult>>({
            queryKey: ["contacts", "list"],
        });
        expect(cached.flatMap(([, d]) => d?.pages.flatMap((p) => p.data) ?? [])).toHaveLength(1);
    });
});
