import { describe, expect, it } from "vitest";
import { verificationTitle } from "./VerificationBadge";

describe("verification provider attribution", () => {
    it.each(["CleanMyList", "MillionVerifier"])("names %s on provider verdicts", (name) => {
        expect(verificationTitle({
            verification_status: "valid",
            verification_source: "provider",
            verification_provider: name.toLowerCase(),
        })).toBe(`Deliverable · checked by ${name}`);
    });
});
