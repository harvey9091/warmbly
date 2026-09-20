import { describe, expect, it } from "vitest";
import { errorMessage } from "./message";

describe("errorMessage", () => {
    it("keeps what the API said, even though it did not arrive as an Error", () => {
        // What the axios interceptor throws: a normalized plain object. The
        // `instanceof Error` check this replaced discarded every one of these.
        const appError = { error: "Bad Request", message: "avatar must be a PNG or JPG", status: 400 };

        expect(errorMessage(appError, "Upload failed.")).toBe("avatar must be a PNG or JPG");
    });

    it("keeps a thrown Error's message too", () => {
        expect(errorMessage(new Error("Couldn't reach the server"), "Upload failed.")).toBe(
            "Couldn't reach the server",
        );
    });

    it("falls back when there is nothing worth showing", () => {
        expect(errorMessage(null, "Upload failed.")).toBe("Upload failed.");
        expect(errorMessage("boom", "Upload failed.")).toBe("Upload failed.");
        expect(errorMessage({ message: "   " }, "Upload failed.")).toBe("Upload failed.");
        expect(errorMessage({ status: 500 }, "Upload failed.")).toBe("Upload failed.");
    });
});
