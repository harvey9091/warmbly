import { beforeEach, describe, expect, it, vi } from "vitest";
import { loadReplyDraft, replyDraftKey, saveReplyDraft } from "./replyDraft";

const draft = { to: [], cc: [], bcc: [], subject: "", body: "Private reply" };

describe("reply draft storage", () => {
    beforeEach(() => {
        setupStorage();
    });

    it("isolates drafts by user, workspace, target and mode", () => {
        const key = replyDraftKey("alice", "org1", "thread1", "message1", "reply");
        expect(saveReplyDraft(key, draft)).toBe(true);
        expect(loadReplyDraft(key)).toEqual(draft);
        for (const other of [
            replyDraftKey("bob", "org1", "thread1", "message1", "reply"),
            replyDraftKey("alice", "org2", "thread1", "message1", "reply"),
            replyDraftKey("alice", "org1", "thread2", "message1", "reply"),
            replyDraftKey("alice", "org1", "thread1", "message2", "reply"),
            replyDraftKey("alice", "org1", "thread1", "message1", "forward"),
        ]) expect(loadReplyDraft(other)).toBeNull();
    });

    it("validates restored recipient arrays without losing intentional empty fields", () => {
        localStorage.setItem("draft", JSON.stringify({ ...draft, to: [null, 12, "valid@example.com"], cc: {}, bcc: [false] }));
        expect(loadReplyDraft("draft")).toEqual({ ...draft, to: ["valid@example.com"] });
        localStorage.setItem("draft", "null");
        expect(loadReplyDraft("draft")).toBeNull();
        localStorage.setItem("draft", "not json");
        expect(loadReplyDraft("draft")).toBeNull();
    });

    it("tolerates browsers that block storage access", () => {
        const spy = vi.spyOn(localStorage, "getItem").mockImplementation(() => { throw new Error("blocked"); });
        expect(loadReplyDraft("draft")).toBeNull();
        spy.mockRestore();
    });
});

function setupStorage() {
    const values = new Map<string, string>();
    vi.mocked(localStorage.getItem).mockImplementation((key: string) => values.get(key) ?? null);
    vi.mocked(localStorage.setItem).mockImplementation((key: string, value: string) => { values.set(key, value); });
    vi.mocked(localStorage.removeItem).mockImplementation((key: string) => { values.delete(key); });
}
