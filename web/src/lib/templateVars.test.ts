import { describe, expect, it } from "vitest";
import { UNSUBSCRIBE_TOKEN, upgradeVariableTokens } from "./templateVars";
import { linkifyUnsubscribe } from "@/components/app/campaigns/sequences/emailPreview";

describe("upgradeVariableTokens", () => {
    it("chips a token in text", () => {
        expect(upgradeVariableTokens("<p>Hi {{.FirstName}}</p>")).toBe('<p>Hi <span data-var="">{{.FirstName}}</span></p>');
    });

    it("leaves a token that is an attribute value alone", () => {
        // A link the author pointed at the unsubscribe variable: wrapping the
        // token in a span here would break the tag.
        const html = `<p><a href="${UNSUBSCRIBE_TOKEN}">no thanks</a></p>`;
        expect(upgradeVariableTokens(html)).toBe(html);
    });

    it("keeps a tag whose attribute contains a >", () => {
        // Reading the quoted ">" as the end of the tag split it, and the href
        // that followed was then wrapped in a chip span.
        const html = `<p><a title="x > y" href="${UNSUBSCRIBE_TOKEN}">no thanks</a></p>`;
        expect(upgradeVariableTokens(html)).toBe(html);
        expect(upgradeVariableTokens(`<p title="a > b">Hi {{.FirstName}}</p>`)).toBe(
            '<p title="a > b">Hi <span data-var="">{{.FirstName}}</span></p>',
        );
    });

    it("is a no-op once the content already carries chips", () => {
        const html = '<p><span data-var="">{{.FirstName}}</span> {{.Company}}</p>';
        expect(upgradeVariableTokens(html)).toBe(html);
    });
});

describe("linkifyUnsubscribe", () => {
    const url = "https://example.com/unsubscribe/preview";

    it("wraps a loose unsubscribe URL in an anchor", () => {
        expect(linkifyUnsubscribe(`<p>Bye. ${url}</p>`)).toBe(`<p>Bye. <a href="${url}">Unsubscribe</a></p>`);
    });

    it("leaves an author's own anchor alone", () => {
        const html = `<p><a href="${url}">no thanks</a></p>`;
        expect(linkifyUnsubscribe(html)).toBe(html);
    });

    it("labels the URL when it is an anchor's own text", () => {
        expect(linkifyUnsubscribe(`<a href="${url}">${url}</a>`)).toBe(`<a href="${url}">Unsubscribe</a>`);
    });

    it("does not rewrite an href behind a quoted > in the same tag", () => {
        const html = `<a title="x > y" href="${url}">read this</a>`;
        expect(linkifyUnsubscribe(html)).toBe(html);
    });

    it("does nothing when the body has no link", () => {
        expect(linkifyUnsubscribe("<p>Hi</p>")).toBe("<p>Hi</p>");
    });
});
