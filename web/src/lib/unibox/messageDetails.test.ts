import { describe, it, expect } from "vitest";
import {
    addressParts,
    folderLabel,
    formatExactTime,
    receivedDiffers,
    recipientsOf,
    relativeTime,
    sameAddress,
    summarizeAddresses,
} from "./messageDetails";

describe("addressParts", () => {
    it("splits every header shape the API hands the UI", () => {
        expect(addressParts("Alice Smith <alice@x.com>")).toEqual({ name: "Alice Smith", address: "alice@x.com" });
        expect(addressParts("Alice Smith (alice@x.com)")).toEqual({ name: "Alice Smith", address: "alice@x.com" });
        expect(addressParts('"Smith, Alice" <alice@x.com>')).toEqual({ name: "Smith, Alice", address: "alice@x.com" });
    });

    it("leaves the name empty for a bare address instead of repeating it", () => {
        expect(addressParts("  alice@x.com ")).toEqual({ name: "", address: "alice@x.com" });
    });
});

describe("summarizeAddresses", () => {
    it("names the first few and counts the rest", () => {
        const list = ["Alice <a@x.com>", "b@x.com", "Carol <c@x.com>", "Dan <d@x.com>"];
        expect(summarizeAddresses(list)).toBe("Alice, b@x.com and 2 more");
        expect(summarizeAddresses(list.slice(0, 2))).toBe("Alice, b@x.com");
        expect(summarizeAddresses([])).toBe("");
    });
});

describe("recipientsOf", () => {
    const email = { id: "m", from: "a@x.com", to: "b@x.com", recipients: ["b@x.com", "c@x.com"], subject: "", date: new Date(), is_seen: true, account_id: "acc" };

    it("prefers the full fetch, then the thread row, then the single address", () => {
        const detail = { to: ["d@x.com"] } as Parameters<typeof recipientsOf>[1];
        expect(recipientsOf(email, detail)).toEqual(["d@x.com"]);
        expect(recipientsOf(email)).toEqual(["b@x.com", "c@x.com"]);
        expect(recipientsOf({ ...email, recipients: [] })).toEqual(["b@x.com"]);
        expect(recipientsOf({ ...email, recipients: undefined, to: "" })).toEqual([]);
    });
});

describe("sameAddress", () => {
    it("compares the bare address, ignoring name and case", () => {
        expect(sameAddress("Alice <Alice@X.com>", "alice@x.com")).toBe(true);
        expect(sameAddress("Alice <alice@x.com>", "Alice <alice@y.com>")).toBe(false);
    });
});

describe("folderLabel", () => {
    it("reads the canonical folders and nothing else", () => {
        expect(folderLabel("inbox")).toBe("Inbox");
        expect(folderLabel("spam")).toBe("Spam");
        expect(folderLabel("")).toBe("");
        expect(folderLabel(undefined)).toBe("");
        expect(folderLabel("outbox")).toBe("");
    });
});

describe("formatExactTime", () => {
    it("names the day, the minute and the zone", () => {
        const s = formatExactTime(new Date("2026-09-16T12:32:00Z"), "en-US", "UTC");
        expect(s).toContain("Wed");
        expect(s).toContain("Sep 16, 2026");
        expect(s).toContain("12:32");
        expect(s).toContain("UTC");
    });
});

describe("relativeTime", () => {
    const now = new Date("2026-09-19T12:00:00Z").getTime();

    it("picks the largest unit that fits", () => {
        expect(relativeTime(new Date("2026-09-16T12:00:00Z"), now, "en-US")).toBe("3 days ago");
        expect(relativeTime(new Date("2026-09-19T09:10:00Z"), now, "en-US")).toBe("2 hours ago");
        expect(relativeTime(new Date("2026-09-19T11:58:00Z"), now, "en-US")).toBe("2 minutes ago");
        expect(relativeTime(new Date("2026-07-19T12:00:00Z"), now, "en-US")).toBe("2 months ago");
    });

    it("says just now under a minute and reads the future too", () => {
        expect(relativeTime(new Date("2026-09-19T11:59:30Z"), now, "en-US")).toBe("just now");
        expect(relativeTime(new Date("2026-09-19T14:00:00Z"), now, "en-US")).toBe("in 2 hours");
    });
});

describe("receivedDiffers", () => {
    const sent = new Date("2026-09-16T12:32:00Z");

    it("ignores clock skew and invalid dates", () => {
        expect(receivedDiffers(sent, new Date("2026-09-16T12:32:40Z"))).toBe(false);
        expect(receivedDiffers(sent, new Date("invalid"))).toBe(false);
    });

    it("flags a real gap either way", () => {
        expect(receivedDiffers(sent, new Date("2026-09-16T12:40:00Z"))).toBe(true);
        expect(receivedDiffers(sent, new Date("2026-09-16T12:20:00Z"))).toBe(true);
    });
});
