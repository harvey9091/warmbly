import type SearchContacts from "./SearchContacts";

// The contacts a bulk action applies to. Either an explicit list of ticked
// ids, or "select all matching": the same search the list ran, minus the rows
// unticked after it. The filter form is what lets an action reach past the
// pages the table has loaded.
export default interface ContactSelection {
    contacts: string[];
    all?: boolean;
    filters?: SearchContacts;
    exclude?: string[];
}

// The explicit form, for the single-row actions that reuse a bulk endpoint.
export function selectionOf(...contacts: string[]): ContactSelection {
    return { contacts };
}
