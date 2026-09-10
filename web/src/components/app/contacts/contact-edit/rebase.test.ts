// Issue #415: a contact who unsubscribed was re-subscribed by lifting their
// suppression on the panel's Overview tab. That is a server-side change to the
// same record the Details tab is holding a draft of, so the draft and the
// record disagreed about `subscribed` and closing the panel asked to discard
// changes the user never made.

import { describe, it, expect } from "vitest";
import {
    fieldsOf,
    idsOf,
    rebase,
    recordFromCF,
    sameCampaigns,
    sameFields,
    sameIDs,
} from "./rebase";

describe("rebase", () => {
    it("adopts the server's new value for a field the user did not touch", () => {
        // The suppression lift re-subscribed them: false -> true, with the
        // draft still sitting on the old value it started from.
        expect(rebase(false, false, true)).toBe(true);
    });

    it("keeps the user's edit when the server moved too", () => {
        expect(rebase("Testing", "Test", "Tested")).toBe("Testing");
    });

    it("keeps the user's edit when they already made the same change", () => {
        expect(rebase(true, false, true)).toBe(true);
    });

    it("is a no-op when nothing moved", () => {
        expect(rebase("Test", "Test", "Test")).toBe("Test");
    });

    it("takes a comparison for values that are not primitives", () => {
        const prev = [{ id: "a", name: "Agency" }];
        const next = [{ id: "a", name: "Agency" }, { id: "b", name: "SaaS" }];
        // Same ids in a different array: still untouched, so adopt.
        expect(rebase([{ id: "a", name: "Agency" }], prev, next, sameCampaigns)).toBe(next);
        // The user removed one: keep their draft.
        const local: typeof prev = [];
        expect(rebase(local, prev, next, sameCampaigns)).toBe(local);
    });
});

describe("sameIDs", () => {
    it("ignores order", () => {
        expect(sameIDs(["a", "b"], ["b", "a"])).toBe(true);
    });

    it("notices an addition and a removal", () => {
        expect(sameIDs(["a"], ["a", "b"])).toBe(false);
        expect(sameIDs(["a", "b"], ["a"])).toBe(false);
    });

    it("treats two empty sets as equal", () => {
        expect(sameIDs([], [])).toBe(true);
    });
});

describe("idsOf", () => {
    it("pulls ids out in order", () => {
        expect(idsOf([{ id: "a" }, { id: "b" }])).toEqual(["a", "b"]);
    });
});

describe("custom fields", () => {
    it("round-trips a record through the row form", () => {
        const record = { industry: "Freight", "job title": "Ops lead" };
        expect(recordFromCF(fieldsOf(record))).toEqual(record);
    });

    it("drops unnamed rows and trims names", () => {
        expect(
            recordFromCF([
                { name: "  industry ", value: "Freight" },
                { name: "   ", value: "orphaned" },
            ]),
        ).toEqual({ industry: "Freight" });
    });

    it("compares rows as the record they save as", () => {
        expect(
            sameFields(
                [{ name: "industry", value: "Freight" }, { name: "", value: "" }],
                [{ name: "industry", value: "Freight" }],
            ),
        ).toBe(true);
        expect(
            sameFields(
                [{ name: "industry", value: "Freight" }],
                [{ name: "industry", value: "Rail" }],
            ),
        ).toBe(false);
    });

    it("survives an undefined record", () => {
        expect(fieldsOf(undefined)).toEqual([]);
    });
});
