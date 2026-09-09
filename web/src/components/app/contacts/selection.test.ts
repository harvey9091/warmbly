import { describe, expect, it } from "vitest";

import * as sel from "./selection";
import type { RowSelection } from "./selection";
import type SearchContacts from "@/lib/api/models/app/contacts/SearchContacts";

const filters: SearchContacts = {
    query: "acme",
    custom_field_filters: [],
    campaign_ids: [],
    sort_by: "created_at",
    reverse: false,
};

// One page of a much larger match set: what the table has actually loaded.
const loaded = ["a", "b", "c"];
const TOTAL = 120;

describe("ticked rows", () => {
    it("counts only what is ticked", () => {
        let s = sel.emptySelection;
        s = sel.toggleRow(s, "a", true);
        s = sel.toggleRow(s, "b", true);
        expect(sel.selectionCount(s, TOTAL)).toBe(2);
        expect(sel.isRowSelected(s, "a")).toBe(true);
        expect(sel.isRowSelected(s, "c")).toBe(false);
        expect(sel.toRequest(s, filters)).toEqual({ contacts: ["a", "b"] });
    });

    it("the header checkbox covers the loaded rows and clears them again", () => {
        let s = sel.toggleLoaded(sel.emptySelection, loaded);
        expect(sel.selectionCount(s, TOTAL)).toBe(3);
        expect(sel.allLoadedSelected(s, loaded)).toBe(true);
        s = sel.toggleLoaded(s, loaded);
        expect(sel.selectionCount(s, TOTAL)).toBe(0);
    });

    it("keeps rows ticked on other pages when the header clears this one", () => {
        let s: RowSelection = { all: false, ids: ["z"], excluded: [] };
        s = sel.toggleLoaded(s, loaded);
        expect(s.ids).toEqual(["z", "a", "b", "c"]);
        s = sel.toggleLoaded(s, loaded);
        expect(s.ids).toEqual(["z"]);
    });
});

describe("select all matching", () => {
    it("is offered only once every loaded row is ticked and more match", () => {
        expect(sel.canSelectAllMatching(sel.emptySelection, loaded, TOTAL)).toBe(false);
        const partial = sel.toggleRow(sel.emptySelection, "a", true);
        expect(sel.canSelectAllMatching(partial, loaded, TOTAL)).toBe(false);
        const all = sel.toggleLoaded(sel.emptySelection, loaded);
        expect(sel.canSelectAllMatching(all, loaded, TOTAL)).toBe(true);
        // Nothing left to reach for when the loaded rows are the whole set.
        expect(sel.canSelectAllMatching(all, loaded, 3)).toBe(false);
    });

    it("counts the search total and travels as the filter", () => {
        const s = sel.selectAllMatching();
        expect(sel.selectionCount(s, TOTAL)).toBe(TOTAL);
        expect(sel.isRowSelected(s, "anything-at-all")).toBe(true);
        expect(sel.toRequest(s, filters)).toEqual({
            contacts: [],
            all: true,
            filters,
            exclude: [],
        });
    });

    it("unticking a row takes it out of the resolved set", () => {
        let s = sel.selectAllMatching();
        s = sel.toggleRow(s, "b", false);
        expect(sel.isRowSelected(s, "b")).toBe(false);
        expect(sel.selectionCount(s, TOTAL)).toBe(TOTAL - 1);
        expect(sel.toRequest(s, filters).exclude).toEqual(["b"]);
        // Ticking it back is not a new selection, it is the exclusion undone.
        s = sel.toggleRow(s, "b", true);
        expect(sel.toRequest(s, filters).exclude).toEqual([]);
        expect(sel.selectionCount(s, TOTAL)).toBe(TOTAL);
    });

    it("never counts below zero when the total moves under it", () => {
        let s = sel.selectAllMatching();
        s = sel.toggleRow(s, "a", false);
        s = sel.toggleRow(s, "b", false);
        expect(sel.selectionCount(s, 1)).toBe(0);
        expect(sel.isEmpty(s, 1)).toBe(true);
    });

    it("the header checkbox puts unticked rows back before it clears", () => {
        let s = sel.selectAllMatching();
        s = sel.toggleRow(s, "a", false);
        // Unchecked because one row on screen is out: click puts it back.
        expect(sel.allLoadedSelected(s, loaded)).toBe(false);
        s = sel.toggleLoaded(s, loaded);
        expect(s.all).toBe(true);
        expect(s.excluded).toEqual([]);
        // Checked now, so the next click clears the whole selection.
        expect(sel.toggleLoaded(s, loaded)).toEqual(sel.emptySelection);
    });

    it("keeps exclusions on rows that are not on screen", () => {
        let s = sel.selectAllMatching();
        s = sel.toggleRow(s, "a", false);
        s = sel.toggleRow(s, "off-screen", false);
        s = sel.toggleLoaded(s, loaded);
        expect(s.excluded).toEqual(["off-screen"]);
    });
});
