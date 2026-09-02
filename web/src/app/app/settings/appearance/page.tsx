// Appearance settings — theme, glassmorphism, and background controls.
//
// All values are persisted client-side via the appearance slice in
// the Zustand store; nothing is sent to the backend.

import React from "react";
import { SunIcon, MoonIcon, MonitorIcon, ImageIcon, TrashIcon } from "lucide-react";
import {
    Row,
    Section,
    SectionShell,
    ToggleRow,
} from "../_components/SectionShell";
import { SelectMenu, type SelectOption } from "@/components/ui/select-menu";
import { NumberInput } from "@/components/ui/field";
import { useAppStore } from "@/stores";
import { usePermission } from "@/hooks/usePermission";
import { NoAccess } from "@/components/layout/NoAccess";

const THEME_OPTIONS: SelectOption[] = [
    { value: "light", label: "Light", icon: <SunIcon className="size-4 text-amber-500" /> },
    { value: "dark", label: "Dark", icon: <MoonIcon className="size-4 text-indigo-400" /> },
    { value: "system", label: "System", icon: <MonitorIcon className="size-4 text-slate-500" /> },
];

const PRESET_OPTIONS: SelectOption[] = [
    { value: "default", label: "Default" },
    { value: "gradient-1", label: "Sky fade" },
    { value: "gradient-2", label: "Lavender" },
    { value: "gradient-3", label: "Mint" },
];

export default function AppearanceSettingsPage() {
    const canManage = usePermission("MANAGE_SETTINGS");
    if (!canManage) return <NoAccess feature="Appearance" permissionLabel="Manage settings" />;
    return <AppearanceSettings />;
}

function AppearanceSettings() {
    const resolvedTheme = useAppStore((state) => state.resolvedTheme)
    const setTheme = useAppStore((state) => state.setTheme)
    const glassmorphismEnabled = useAppStore((state) => state.glassmorphismEnabled)
    const glassOpacity = useAppStore((state) => state.glassOpacity)
    const glassBlur = useAppStore((state) => state.glassBlur)
    const backgroundPreset = useAppStore((state) => state.backgroundPreset)
    const backgroundImage = useAppStore((state) => state.backgroundImage)
    const backgroundBlur = useAppStore((state) => state.backgroundBlur)
    const backgroundOpacity = useAppStore((state) => state.backgroundOpacity)
    const setGlassmorphismEnabled = useAppStore((state) => state.setGlassmorphismEnabled)
    const setGlassOpacity = useAppStore((state) => state.setGlassOpacity)
    const setGlassBlur = useAppStore((state) => state.setGlassBlur)
    const setBackgroundPreset = useAppStore((state) => state.setBackgroundPreset)
    const setBackgroundImage = useAppStore((state) => state.setBackgroundImage)
    const setBackgroundBlur = useAppStore((state) => state.setBackgroundBlur)
    const setBackgroundOpacity = useAppStore((state) => state.setBackgroundOpacity)

    const appearance = {
        glassmorphismEnabled,
        glassOpacity,
        glassBlur,
        backgroundPreset,
        backgroundImage,
        backgroundBlur,
        backgroundOpacity,
        setGlassmorphismEnabled,
        setGlassOpacity,
        setGlassBlur,
        setBackgroundPreset,
        setBackgroundImage,
        setBackgroundBlur,
        setBackgroundOpacity,
    }

    const themeValue = THEME_OPTIONS.find((o) => o.value === resolvedTheme)?.value ?? "system";

    return (
        <SectionShell
            title="Appearance"
            description="Customize how Warmbly looks for you. Changes apply instantly."
        >
            <Section
                eyebrow="Theme"
                description="Choose between light, dark, or follow your system preference."
            >
                <Row label="Appearance">
                    <SelectMenu
                        value={themeValue}
                        onChange={(v) => setTheme(v as "light" | "dark" | "system")}
                        options={THEME_OPTIONS}
                        aria-label="Theme"
                        minWidth={160}
                        align="end"
                    />
                </Row>
            </Section>

            <Section
                eyebrow="Glassmorphism"
                description="Add a subtle frosted-glass effect to panels, cards, and dialogs. Off by default."
            >
                <ToggleRow
                    label="Enable glassmorphism"
                    description="Applies a translucent, blurred surface to the sidebar, navigation, cards, and dialogs."
                    checked={appearance.glassmorphismEnabled}
                    onChange={appearance.setGlassmorphismEnabled}
                />
                {appearance.glassmorphismEnabled && (
                    <>
                        <Row
                            label="Glass opacity"
                            description={`${appearance.glassOpacity}% transparency. Lower is more opaque.`}
                        >
                            <NumberInput
                                value={appearance.glassOpacity}
                                onChange={appearance.setGlassOpacity}
                                min={5}
                                max={60}
                                step={1}
                                className="w-20"
                            />
                        </Row>
                        <Row
                            label="Glass blur"
                            description={`${appearance.glassBlur}px blur strength.`}
                        >
                            <NumberInput
                                value={appearance.glassBlur}
                                onChange={appearance.setGlassBlur}
                                min={0}
                                max={40}
                                step={1}
                                className="w-20"
                            />
                        </Row>
                    </>
                )}
            </Section>

            <Section
                eyebrow="Background"
                description="Set a background image behind the application. Stored locally in your browser."
            >
                <Row label="Background">
                    <SelectMenu
                        value={appearance.backgroundPreset}
                        onChange={(v) => {
                            if (v === "default") {
                                appearance.setBackgroundPreset("default");
                                appearance.setBackgroundImage("");
                            } else {
                                appearance.setBackgroundPreset(v as typeof appearance.backgroundPreset);
                            }
                        }}
                        options={PRESET_OPTIONS}
                        aria-label="Background preset"
                        minWidth={160}
                        align="end"
                    />
                </Row>
                <Row label="Upload image">
                    <BackgroundImageUploader
                        current={appearance.backgroundImage}
                        onSelect={(url) => {
                            appearance.setBackgroundImage(url);
                            if (url) appearance.setBackgroundPreset("default");
                        }}
                        onClear={() => {
                            appearance.setBackgroundImage("");
                            appearance.setBackgroundPreset("default");
                        }}
                    />
                </Row>
                <Row
                    label="Background blur"
                    description={`${appearance.backgroundBlur}px. Only affects the background image, not the UI.`}
                >
                    <NumberInput
                        value={appearance.backgroundBlur}
                        onChange={appearance.setBackgroundBlur}
                        min={0}
                        max={40}
                        step={1}
                        className="w-20"
                    />
                </Row>
                <Row
                    label="Background opacity"
                    description={`${appearance.backgroundOpacity}%. Reduce to make the background less visible.`}
                >
                    <NumberInput
                        value={appearance.backgroundOpacity}
                        onChange={appearance.setBackgroundOpacity}
                        min={0}
                        max={100}
                        step={1}
                        className="w-20"
                    />
                </Row>
            </Section>
        </SectionShell>
    );
}

function BackgroundImageUploader({
    current,
    onSelect,
    onClear,
}: {
    current: string;
    onSelect: (url: string) => void;
    onClear: () => void;
}) {
    const inputRef = React.useRef<HTMLInputElement>(null);
    const [preview, setPreview] = React.useState<string | null>(current || null);

    React.useEffect(() => {
        setPreview(current || null);
    }, [current]);

    React.useEffect(() => {
        return () => {
            if (preview && preview !== current) URL.revokeObjectURL(preview);
        };
    }, [preview, current]);

    function onPicked(e: React.ChangeEvent<HTMLInputElement>) {
        const f = e.target.files?.[0];
        e.target.value = "";
        if (!f) return;
        if (!f.type.startsWith("image/")) return;
        const url = URL.createObjectURL(f);
        setPreview(url);
        onSelect(url);
    }

    const displayUrl = preview || current;

    return (
        <div className="flex items-center gap-3">
            <input
                ref={inputRef}
                type="file"
                accept="image/*"
                onChange={onPicked}
                className="hidden"
                aria-hidden="true"
            />
            {displayUrl && (
                <div className="size-10 rounded-md border border-slate-200 overflow-hidden shrink-0 bg-slate-50">
                    <img
                        src={displayUrl}
                        alt=""
                        className="w-full h-full object-cover"
                    />
                </div>
            )}
            <div className="flex items-center gap-1.5">
                <button
                    type="button"
                    onClick={() => inputRef.current?.click()}
                    className="h-7 px-2.5 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 hover:text-slate-900 transition-colors inline-flex items-center gap-1.5"
                >
                    <ImageIcon className="w-3 h-3" />
                    {displayUrl ? "Replace" : "Upload"}
                </button>
                {displayUrl && (
                    <button
                        type="button"
                        onClick={() => {
                            if (preview && preview !== current) URL.revokeObjectURL(preview);
                            setPreview(null);
                            onClear();
                        }}
                        className="h-7 px-2.5 rounded-md text-[12px] text-slate-500 hover:text-red-700 hover:bg-red-50 transition-colors inline-flex items-center gap-1.5"
                    >
                        <TrashIcon className="w-3 h-3" />
                        Remove
                    </button>
                )}
            </div>
        </div>
    );
}
