// Row selection for the contacts table, its campaign Leads and segment
// members variants.
//
// A selection is one of two things and the difference matters at every call
// site: a list of rows the user ticked, or "everything the current search
// matches" minus the rows unticked afterwards. Only the second reaches past
// the pages the table has loaded, and only the server can resolve it, so it
// travels as the search itself.

import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";
import type ContactSelection from "@/lib/api/models/app/contacts/ContactSelection";

export interface RowSelection {
    /** Every contact the current search matches, rather than `ids`. */
    all: boolean;
    /** Ticked rows. Empty while `all` is set. */
    ids: string[];
    /** Rows unticked after a select-all. Empty while `all` is not set. */
    excluded: string[];
}

export const emptySelection: RowSelection = { all: false, ids: [], excluded: [] };

export function isRowSelected(s: RowSelection, id: string): boolean {
    return s.all ? !s.excluded.includes(id) : s.ids.includes(id);
}

/** How many contacts the selection covers. `total` is the search's own total. */
export function selectionCount(s: RowSelection, total: number): number {
    return s.all ? Math.max(total - s.excluded.length, 0) : s.ids.length;
}

export function isEmpty(s: RowSelection, total: number): boolean {
    return selectionCount(s, total) === 0;
}

export function toggleRow(s: RowSelection, id: string, on: boolean): RowSelection {
    if (s.all) {
        return { ...s, excluded: on ? s.excluded.filter((x) => x !== id) : [...s.excluded, id] };
    }
    return { ...s, ids: on ? [...s.ids, id] : s.ids.filter((x) => x !== id) };
}

/** Whether every row on screen is selected: the header checkbox's state. */
export function allLoadedSelected(s: RowSelection, loaded: string[]): boolean {
    return loaded.length > 0 && loaded.every((id) => isRowSelected(s, id));
}

/**
 * The header checkbox, which reads the rows on screen. In select-all mode an
 * unchecked box means some of them were unticked, so it puts those back rather
 * than dropping a selection the user never asked to lose; a checked one clears.
 */
export function toggleLoaded(s: RowSelection, loaded: string[]): RowSelection {
    if (s.all) {
        if (allLoadedSelected(s, loaded)) return emptySelection;
        return { ...s, excluded: s.excluded.filter((id) => !loaded.includes(id)) };
    }
    if (allLoadedSelected(s, loaded)) {
        return { ...s, ids: s.ids.filter((id) => !loaded.includes(id)) };
    }
    return { ...s, ids: Array.from(new Set([...s.ids, ...loaded])) };
}

export function selectAllMatching(): RowSelection {
    return { all: true, ids: [], excluded: [] };
}

/**
 * Whether the "select all N matching" bar has anything to offer: every loaded
 * row is ticked and more rows match than are ticked.
 */
export function canSelectAllMatching(s: RowSelection, loaded: string[], total: number): boolean {
    return !s.all && allLoadedSelected(s, loaded) && total > s.ids.length;
}

/** The wire shape every bulk endpoint takes. */
export function toRequest(s: RowSelection, filters: SearchContacts): ContactSelection {
    if (!s.all) return { contacts: s.ids };
    return { contacts: [], all: true, filters, exclude: s.excluded };
}
