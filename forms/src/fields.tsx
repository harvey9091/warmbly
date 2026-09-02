// One controlled input per field type, class-for-class with the page
// stylesheet. Layout blocks (heading, paragraph, divider) render in
// FormRenderer; hidden fields never render at all.

import type { FormField } from "./api";

export type AnswerValue = string | string[] | boolean;

export function FieldControl({
    field,
    value,
    onChange,
    onBlur,
}: {
    field: FormField;
    value: AnswerValue;
    onChange: (v: AnswerValue) => void;
    onBlur: () => void;
}) {
    const id = `f-${field.id}`;
    const str = typeof value === "string" ? value : "";

    switch (field.type) {
        case "textarea":
            return (
                <textarea
                    id={id}
                    rows={field.rows || 4}
                    placeholder={field.placeholder}
                    value={str}
                    onChange={(e) => onChange(e.target.value)}
                    onBlur={onBlur}
                />
            );
        case "select":
            return (
                <select id={id} value={str} onChange={(e) => onChange(e.target.value)} onBlur={onBlur}>
                    <option value="">{field.placeholder || "Select…"}</option>
                    {(field.options ?? []).map((o) => (
                        <option key={o} value={o}>
                            {o}
                        </option>
                    ))}
                </select>
            );
        case "radio":
            return (
                <div className="opts">
                    {(field.options ?? []).map((o) => (
                        <label key={o} className="opt">
                            <input
                                type="radio"
                                name={field.id}
                                value={o}
                                checked={str === o}
                                onChange={() => onChange(o)}
                                onBlur={onBlur}
                            />{" "}
                            {o}
                        </label>
                    ))}
                </div>
            );
        case "checkboxes": {
            const arr = Array.isArray(value) ? value : [];
            const toggle = (o: string) => onChange(arr.includes(o) ? arr.filter((x) => x !== o) : [...arr, o]);
            return (
                <div className="opts">
                    {(field.options ?? []).map((o) => (
                        <label key={o} className="opt">
                            <input type="checkbox" checked={arr.includes(o)} onChange={() => toggle(o)} onBlur={onBlur} />{" "}
                            {o}
                        </label>
                    ))}
                </div>
            );
        }
        case "checkbox":
            return (
                <label className="opt">
                    <input
                        type="checkbox"
                        id={id}
                        checked={value === true}
                        onChange={(e) => onChange(e.target.checked)}
                        onBlur={onBlur}
                    />{" "}
                    {field.placeholder}
                </label>
            );
        case "email":
            return (
                <input
                    type="email"
                    id={id}
                    placeholder={field.placeholder}
                    autoComplete="email"
                    value={str}
                    onChange={(e) => onChange(e.target.value)}
                    onBlur={onBlur}
                />
            );
        case "phone":
            return (
                <input
                    type="tel"
                    id={id}
                    placeholder={field.placeholder}
                    autoComplete="tel"
                    value={str}
                    onChange={(e) => onChange(e.target.value)}
                    onBlur={onBlur}
                />
            );
        case "number":
            return (
                <input
                    type="number"
                    id={id}
                    placeholder={field.placeholder}
                    value={str}
                    onChange={(e) => onChange(e.target.value)}
                    onBlur={onBlur}
                />
            );
        case "date":
            return <input type="date" id={id} value={str} onChange={(e) => onChange(e.target.value)} onBlur={onBlur} />;
        default:
            return (
                <input
                    type="text"
                    id={id}
                    placeholder={field.placeholder}
                    value={str}
                    onChange={(e) => onChange(e.target.value)}
                    onBlur={onBlur}
                />
            );
    }
}
