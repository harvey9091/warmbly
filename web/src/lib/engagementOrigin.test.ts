import { describe, expect, it } from "vitest";
import { deviceLabel, hiddenReason, originSummary, readerLabel } from "./engagementOrigin";

describe("originSummary", () => {
    it("names the device and the app", () => {
        expect(originSummary({ client: "Apple Mail", client_type: "app", device_type: "mobile", os: "iOS" })).toBe(
            "iPhone · Apple Mail app",
        );
        expect(originSummary({ client: "Outlook", client_type: "app", device_type: "desktop", os: "Windows" })).toBe(
            "Windows PC · Outlook app",
        );
    });
    it("says a proxy hid the device instead of guessing one", () => {
        expect(originSummary({ client: "Gmail", device_hidden: true })).toBe("Gmail · device hidden");
        expect(hiddenReason({ client: "Gmail", device_hidden: true })).toMatch(/Google's servers/);
        expect(hiddenReason({ client: "Gmail" })).toBe("");
    });
    it("reads an unnamed browser open as webmail", () => {
        expect(originSummary({ client_type: "webmail", browser: "Chrome", device_type: "desktop", os: "macOS" })).toBe(
            "Mac · Webmail in Chrome",
        );
    });
    it("names the browser a link opened in, never webmail", () => {
        expect(readerLabel({ client_type: "webmail", browser: "Safari" }, "click")).toBe("Safari");
        expect(originSummary({ browser: "Safari", device_type: "mobile", os: "iOS" }, "click")).toBe("iPhone · Safari");
    });
    it("says nothing when nothing is known", () => {
        expect(originSummary({})).toBe("");
        expect(deviceLabel({ device_type: "unknown" })).toBe("");
    });
    it("tells tablets apart", () => {
        expect(deviceLabel({ device_type: "tablet", os: "iOS" })).toBe("iPad");
        expect(deviceLabel({ device_type: "tablet", os: "Android" })).toBe("Android tablet");
    });
});
