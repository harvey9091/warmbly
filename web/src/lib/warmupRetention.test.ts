import { describe, expect, it } from "vitest";
import { clampWarmupRetentionDays } from "./warmupRetention";

describe("clampWarmupRetentionDays", () => {
    it("keeps 0 as the instance setting", () => {
        expect(clampWarmupRetentionDays(0)).toBe(0);
        expect(clampWarmupRetentionDays(-4)).toBe(0);
        expect(clampWarmupRetentionDays(Number.NaN)).toBe(0);
    });
    it("never yields 1 or 2, which the API refuses", () => {
        expect(clampWarmupRetentionDays(1)).toBe(0); // stepping down from 3
        expect(clampWarmupRetentionDays(2)).toBe(3); // stepping up from 0, or typed
    });
    it("keeps the band and drops fractions", () => {
        expect(clampWarmupRetentionDays(3)).toBe(3);
        expect(clampWarmupRetentionDays(14.9)).toBe(14);
        expect(clampWarmupRetentionDays(9999)).toBe(3650);
    });
});
