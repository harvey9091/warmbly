// The design engine lives in designCore.ts (mirrored with the builder
// canvas); this file only paints a resolved design onto the document.

import { designVars, ensureFont, type ResolvedDesign } from "./designCore";

export {
    resolveDesign,
    splitPages,
    focusSteps,
    pageIndexOf,
    type ResolvedDesign,
    type FormPage,
} from "./designCore";

// applyDesign paints the resolved theme onto the document as CSS custom
// properties; form-theme.css reads nothing but these variables.
export function applyDesign(r: ResolvedDesign) {
    const s = document.documentElement.style;
    for (const [name, value] of Object.entries(designVars(r))) {
        s.setProperty(name, value);
    }
    ensureFont(r);
}
