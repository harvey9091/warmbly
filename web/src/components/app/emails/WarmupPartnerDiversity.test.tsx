import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import WarmupPartnerDiversity from "./WarmupPartnerDiversity";

describe("WarmupPartnerDiversity", () => {
    it("shows confirmed partner reach and the single-workspace warning", () => {
        render(
            <WarmupPartnerDiversity
                health={{
                    partner_mailboxes_7d: 6,
                    partner_domains_7d: 4,
                    partner_organizations_7d: 1,
                }}
            />,
        );

        expect(screen.getByText("6")).toBeTruthy();
        expect(screen.getByText("4")).toBeTruthy();
        expect(screen.getByText("Every partner this week was in a single workspace.", { exact: false })).toBeTruthy();
    });

    it("renders nothing when an older cloud response has no diversity fields", () => {
        const { container } = render(<WarmupPartnerDiversity health={{}} />);
        expect(container.firstChild).toBeNull();
    });
});
