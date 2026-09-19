import { beforeEach, describe, expect, it, vi } from "vitest";
import Client from "./Client";
import Request from "./Request";
import getToken from "@/lib/helper/getToken";
import refreshTokenFn from "./auth/refreshToken";
import { AuthError } from "@/lib/errors/auth";
import { SESSION_ENDED_EVENT } from "@/lib/auth";

vi.mock("./auth/refreshToken", () => ({
    default: vi.fn(),
}));

// Counted rather than asserted on localStorage alone: ending a session has to
// say so, or nothing navigates and the app sits on a page it cannot load.
let endedSessions = 0;
window.addEventListener(SESSION_ENDED_EVENT, () => {
    endedSessions += 1;
});

vi.mock("./Client", () => ({
    default: {
        request: vi.fn(),
    },
}));

vi.mock("@/lib/helper/getToken", () => ({
    default: vi.fn(),
}));

describe("Request", () => {
    beforeEach(() => {
        vi.clearAllMocks();
        endedSessions = 0;
        vi.mocked(getToken).mockReturnValue({
            access_token: "access-token",
            access_token_expires_at: new Date(Date.now() + 60_000),
            refresh_token: "refresh-token",
            refresh_token_expires_at: new Date(Date.now() + 120_000),
        });
    });

    it("preserves blob responses for browser downloads", async () => {
        const blob = new Blob(["email\nlead@example.com\n"], { type: "text/csv" });
        vi.mocked(Client.request).mockResolvedValue({ data: blob });

        const result = await Request<Blob>({
            method: "POST",
            url: "/contacts/export",
            authorization: true,
            responseType: "blob",
        });

        expect(result).toBe(blob);
        expect(result).toBeInstanceOf(Blob);
    });

    it("continues reviving timestamps in JSON responses", async () => {
        vi.mocked(Client.request).mockResolvedValue({
            data: {
                created_at: "2026-09-17T12:34:56.000Z",
                nested: [{ updated_at: "2026-09-18T01:02:03.000Z" }],
                label: "2026-09-17",
            },
        });

        const result = await Request<{
            created_at: Date;
            nested: Array<{ updated_at: Date }>;
            label: string;
        }>({
            method: "GET",
            url: "/contacts/example",
            authorization: true,
        });

        expect(result.created_at).toEqual(new Date("2026-09-17T12:34:56.000Z"));
        expect(result.nested[0]?.updated_at).toEqual(new Date("2026-09-18T01:02:03.000Z"));
        expect(result.label).toBe("2026-09-17");
    });

    // The database ran out of connection slots, /auth/refresh answered 500 for
    // about a minute, and everyone whose access token expired in that minute
    // was signed out and could not get back in: the refresh token the server
    // had never refused was deleted along with the session.
    describe("when the access token has expired", () => {
        const expiredAccess = () => {
            vi.mocked(getToken).mockReturnValue({
                access_token: "access-token",
                access_token_expires_at: new Date(Date.now() - 60_000),
                refresh_token: "refresh-token",
                refresh_token_expires_at: new Date(Date.now() + 120_000),
            });
        };

        beforeEach(() => {
            expiredAccess();
        });

        it("keeps the session when the refresh only failed to get an answer", async () => {
            vi.mocked(refreshTokenFn).mockRejectedValue({
                error: "Internal Server Error",
                message: "Something went wrong.",
                status: 500,
            });

            await expect(
                Request({ method: "GET", url: "/emails", authorization: true }),
            ).rejects.not.toBeInstanceOf(AuthError);

            expect(endedSessions).toBe(0);
        });

        it("keeps the session when the refresh never reached the server", async () => {
            vi.mocked(refreshTokenFn).mockRejectedValue({
                error: "Network Error",
                message: "Please check your connection.",
            });

            await expect(
                Request({ method: "GET", url: "/emails", authorization: true }),
            ).rejects.not.toBeInstanceOf(AuthError);

            expect(endedSessions).toBe(0);
        });

        it("ends the session when the server refuses the refresh token", async () => {
            vi.mocked(refreshTokenFn).mockRejectedValue({
                error: "Unauthorized",
                message: "Your session is invalid or expired.",
                status: 401,
            });

            await expect(
                Request({ method: "GET", url: "/emails", authorization: true }),
            ).rejects.toBeInstanceOf(AuthError);

            expect(endedSessions).toBe(1);
        });
    });
});
