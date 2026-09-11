import { describe, expect, it } from "vitest";
import { bareEmail, nameFromAddr, wrappedEmail } from "./emailAddress";

// The three shapes the API actually stores (see the header comment), pinned
// so the reply composer's seeded To passes its own validator for each.
describe("emailAddress", () => {
    it("parses the IMAP sync's parenthesised form", () => {
        const s = "Centous Support (support@centous.com)";
        expect(bareEmail(s)).toBe("support@centous.com");
        expect(nameFromAddr(s)).toBe("Centous Support");
    });

    it("parses the RFC angle-bracket form, quoted or not", () => {
        expect(bareEmail('"Jane Doe" <jane@x.com>')).toBe("jane@x.com");
        expect(nameFromAddr('"Jane Doe" <jane@x.com>')).toBe("Jane Doe");
        expect(nameFromAddr("Jane Doe <jane@x.com>")).toBe("Jane Doe");
    });

    it("passes a bare address through and names it by itself", () => {
        expect(wrappedEmail("jane@x.com")).toBeNull();
        expect(bareEmail("  jane@x.com ")).toBe("jane@x.com");
        expect(nameFromAddr("jane@x.com")).toBe("jane@x.com");
    });

    it("falls back to the address when the name is empty", () => {
        expect(nameFromAddr(" (noreply-dmarc-support@google.com)")).toBe("noreply-dmarc-support@google.com");
    });
});
