import { describe, expect, it } from "vitest";
import { nameError } from "./displayName";

describe("nameError", () => {
    it.each(["Ada", "  Mary   Ann ", "O'Brien-Smith", "J.R.R. Tolkien", "St. John", "Zoë", "李小龙"])(
        "accepts %s",
        (v) => expect(nameError("Name", v, "person")).toBeNull(),
    );

    it.each([
        "https://www.google.com",
        "google.com",
        "www.example",
        "a@b",
        "javascript:alert(1)",
        "mailto:x",
        "ｇｏｏｇｌｅ．ｃｏｍ",
        "google。com",
        "10.0.0.1",
    ])("refuses the link %s", (v) => expect(nameError("Name", v, "person")).toMatch(/cannot contain a link/));

    it.each(["Ada‮evil", "Ada​", "<b>Ada</b>", "Ź́́́́"])(
        "refuses the characters in %s",
        (v) => expect(nameError("Name", v, "person")).toMatch(/not allowed/),
    );

    it("bounds length by kind", () => {
        expect(nameError("Name", "a".repeat(51), "person")).toMatch(/50/);
        expect(nameError("Name", "a".repeat(64), "workspace")).toBeNull();
    });

    it("allows an empty optional name", () => {
        expect(nameError("Name", " ", "person", true)).toBeNull();
        expect(nameError("Name", " ", "person")).toMatch(/required/);
    });

    it("lets a workspace be only digits", () => {
        expect(nameError("Workspace name", "3000", "workspace")).toBeNull();
        expect(nameError("Name", "3000", "person")).toMatch(/letter/);
    });
});
