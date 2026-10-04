import { describe, expect, it } from "vitest";
import { defaultScheduleTimezone, timezoneOptions } from "./timezone";

const list = [
    { name: "Pacific/Honolulu", display_name: "(UTC-10:00) Pacific/Honolulu" },
    { name: "America/New_York", display_name: "(UTC-04:00) America/New_York" },
];

describe("defaultScheduleTimezone", () => {
    it("prefers the workspace zone", () => {
        expect(defaultScheduleTimezone("Europe/Paris")).toBe("Europe/Paris");
    });
    it("falls back to the browser zone, never the first list entry", () => {
        const zone = defaultScheduleTimezone("");
        expect(zone).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC");
        expect(zone).not.toBe("");
    });
});

describe("timezoneOptions", () => {
    it("keeps the list as is when the current value is on it", () => {
        expect(timezoneOptions(list, "America/New_York").map((o) => o.value)).toEqual(["Pacific/Honolulu", "America/New_York"]);
    });
    it("adds a zone the list does not carry so the choice is never blank", () => {
        const options = timezoneOptions(list, "Europe/Budapest");
        expect(options[0].value).toBe("Europe/Budapest");
        expect(options[0].label).toMatch(/^\(UTC[+-]\d\d:\d\d\) Europe\/Budapest$/);
    });
    it("ignores empty extras and tolerates an unloaded list", () => {
        expect(timezoneOptions(undefined, "", null, undefined)).toEqual([]);
    });
});
