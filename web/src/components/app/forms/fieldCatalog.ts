// The builder's palette: every block a form can hold, with a factory that
// mints a fresh field with a unique, stable id.

import type { LucideIcon } from "lucide-react";
import {
    AlignLeftIcon,
    AtSignIcon,
    CalendarIcon,
    CheckSquareIcon,
    ChevronDownSquareIcon,
    CircleDotIcon,
    EyeOffIcon,
    HashIcon,
    HeadingIcon,
    ListChecksIcon,
    MinusIcon,
    PhoneIcon,
    SeparatorHorizontalIcon,
    TextIcon,
    TypeIcon,
} from "lucide-react";
import type { FormField, FormFieldType } from "@/lib/api/models/app/forms/Form";

export interface PaletteItem {
    type: FormFieldType;
    label: string;
    icon: LucideIcon;
    group: "Fields" | "Layout";
}

export const PALETTE: PaletteItem[] = [
    { type: "text", label: "Text", icon: TypeIcon, group: "Fields" },
    { type: "email", label: "Email", icon: AtSignIcon, group: "Fields" },
    { type: "phone", label: "Phone", icon: PhoneIcon, group: "Fields" },
    { type: "textarea", label: "Long text", icon: AlignLeftIcon, group: "Fields" },
    { type: "number", label: "Number", icon: HashIcon, group: "Fields" },
    { type: "select", label: "Dropdown", icon: ChevronDownSquareIcon, group: "Fields" },
    { type: "radio", label: "Radio buttons", icon: CircleDotIcon, group: "Fields" },
    { type: "checkboxes", label: "Checkbox group", icon: ListChecksIcon, group: "Fields" },
    { type: "checkbox", label: "Single checkbox", icon: CheckSquareIcon, group: "Fields" },
    { type: "date", label: "Date", icon: CalendarIcon, group: "Fields" },
    { type: "hidden", label: "Hidden field", icon: EyeOffIcon, group: "Fields" },
    { type: "heading", label: "Heading", icon: HeadingIcon, group: "Layout" },
    { type: "paragraph", label: "Text block", icon: TextIcon, group: "Layout" },
    { type: "divider", label: "Divider", icon: MinusIcon, group: "Layout" },
    { type: "page_break", label: "Page break", icon: SeparatorHorizontalIcon, group: "Layout" },
];

const DEFAULT_LABELS: Partial<Record<FormFieldType, string>> = {
    text: "Text",
    email: "Email",
    phone: "Phone",
    textarea: "Message",
    number: "Number",
    select: "Pick one",
    radio: "Pick one",
    checkboxes: "Pick any",
    checkbox: "Checkbox",
    date: "Date",
    hidden: "Hidden field",
    heading: "Heading",
};

let counter = 0;

/** Mints a new field. Ids only need to be unique inside one form. */
export function newField(type: FormFieldType): FormField {
    counter += 1;
    const id = `${type.replace(/[^a-z0-9]/g, "")}-${Date.now().toString(36)}${counter.toString(36)}`;
    const f: FormField = { id, type, label: DEFAULT_LABELS[type] ?? "", required: false };
    if (type === "select" || type === "radio" || type === "checkboxes") f.options = ["Option 1", "Option 2"];
    if (type === "email") f.map_to = "email";
    if (type === "paragraph") f.value = "Write something…";
    if (type === "checkbox") f.placeholder = "I agree";
    if (type === "textarea") f.rows = 4;
    return f;
}

export function paletteFor(type: FormFieldType): PaletteItem | undefined {
    return PALETTE.find((p) => p.type === type);
}
