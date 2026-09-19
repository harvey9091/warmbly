import { describe, expect, it } from "vitest";
import { DEFAULT_COLUMNS, builtinColumns, customColumnId, resolveColumns } from "./columns";

describe("resolveColumns", () => {
    it("shows the default layout with Name first when nothing is saved", () => {
        const { visible, available } = resolveColumns("contacts", undefined, ["Industry"]);
        expect(visible.map((c) => c.id)).toEqual(["name", ...DEFAULT_COLUMNS.contacts]);
        // Everything the view knows and is not shown is offered, custom fields last.
        expect(available.map((c) => c.id)).toEqual(["updated_at", customColumnId("Industry")]);
    });

    it("follows the saved order and keeps Name pinned first", () => {
        const { visible } = resolveColumns("contacts", ["phone", "name", "company"], []);
        expect(visible.map((c) => c.id)).toEqual(["name", "phone", "company"]);
    });

    it("renders a saved custom field even when no contact carries it any more", () => {
        const { visible, available } = resolveColumns("contacts", ["custom:Old Field"], ["Industry"]);
        expect(visible.map((c) => c.id)).toEqual(["name", "custom:Old Field"]);
        expect(visible[1].label).toBe("Old Field");
        expect(visible[1].custom).toBe("Old Field");
        expect(available.map((c) => c.id)).toContain(customColumnId("Industry"));
        expect(available.map((c) => c.id)).not.toContain("custom:Old Field");
    });

    it("drops ids the view does not know and duplicates", () => {
        const { visible } = resolveColumns("campaign_leads", ["status", "opened", "opened", "sender"], []);
        // "status" belongs to the contacts view; the Leads view has "progress".
        expect(visible.map((c) => c.id)).toEqual(["name", "opened", "sender"]);
    });

    it("gives every non-Name column a width, because the table is table-fixed", () => {
        for (const view of ["contacts", "campaign_leads"] as const) {
            for (const col of builtinColumns(view)) {
                if (col.locked) continue;
                expect(col.width, `${view}/${col.id}`).not.toBe("");
            }
        }
    });

    it("reads a custom field's value and sorts on it as text", () => {
        const { visible } = resolveColumns("contacts", ["custom:Industry"], []);
        const col = visible[1];
        expect(col.sortKey).toBe("custom:Industry");
        expect(col.sortAsc).toBe(true);
    });
});
