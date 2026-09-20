import { describe, expect, it } from "vitest";
import { passkeyChallengeUnavailable } from "./passkey";

describe("passkeyChallengeUnavailable", () => {
    it("treats a throttled address as something waiting fixes", () => {
        // The sign-in page asks for a challenge on load, so a shared address
        // can be refused one without anything being wrong.
        const throttled = Object.assign(new Error("Too many passkey sign-in requests from this address."), {
            status: 429,
            code: "rate_limit_exceeded",
        });

        expect(passkeyChallengeUnavailable(throttled)).toBe(true);
    });

    it("treats a request that never got an answer the same way", () => {
        expect(passkeyChallengeUnavailable(new TypeError("Failed to fetch"))).toBe(true);
    });

    it("still reports a server that answered with a fault", () => {
        const broken = Object.assign(new Error("Something went wrong."), { status: 500 });

        expect(passkeyChallengeUnavailable(broken)).toBe(false);
    });

    it("still reports a refusal that is not a throttle", () => {
        const refused = Object.assign(new Error("Passkeys are not enabled here."), { status: 404 });

        expect(passkeyChallengeUnavailable(refused)).toBe(false);
    });
});
