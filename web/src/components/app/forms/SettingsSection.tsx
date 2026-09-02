// Section — the builder's settings block: a full-width heading with its
// explanation, then its controls in a responsive grid beneath. Mirrors the
// campaign preferences page (header above, controls below, hairline between
// sections) and spreads controls across the available width instead of
// stranding them in one narrow column on a wide screen.

import type React from "react";

export default function Section({
    title,
    description,
    children,
}: {
    title: string;
    description?: string;
    children: React.ReactNode;
}) {
    return (
        <section className="py-6 first:pt-0">
            <div className="mb-4">
                <h2 className="text-[13.5px] font-semibold text-slate-900">{title}</h2>
                {description && (
                    <p className="text-[11.5px] text-slate-400 mt-1 leading-relaxed max-w-3xl">{description}</p>
                )}
            </div>
            <div className="grid gap-x-8 gap-y-5 md:grid-cols-2 xl:grid-cols-3">{children}</div>
        </section>
    );
}
