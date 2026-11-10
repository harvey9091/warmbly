import React from "react";
import { useUserProfile } from "@/hooks/context/user";
import { FieldError, NumberInput, TextInput } from "@/components/ui/field";
import useUpdateProfile from "@/lib/api/hooks/auth/useUpdateProfile";
import useUpdateSendPreferences from "@/lib/api/hooks/auth/useUpdateSendPreferences";
import { AvatarUploader } from "@/components/app/avatar/AvatarUploader";
import {
    useDeleteUserAvatar,
    useUploadUserAvatar,
} from "@/lib/api/hooks/app/avatar/useUserAvatar";
import { Row, Section, SectionShell, initials } from "../_components/SectionShell";
import SaveStatus from "../_components/SaveStatus";
import { useAutosave, type AutosaveStatus } from "@/hooks/useAutosave";
import { useRegisterUnsaved } from "@/hooks/context/unsaved";
import { PERSON_NAME_MAX, nameError, normalizeName } from "@/lib/displayName";
import TimezonesSection from "./TimezonesSection";
import { ThemeCards } from "@/components/app/theme/ThemePicker";
import { useAppStore } from "@/stores";

// Header indicator priority when two autosaves share one SaveStatus.
function combineStatus(a: AutosaveStatus, b: AutosaveStatus): AutosaveStatus {
    for (const s of ["saving", "error", "saved"] as const) {
        if (a === s || b === s) return s;
    }
    return "idle";
}

export default function ProfileSettingsPage() {
    const { user } = useUserProfile();
    const [firstName, setFirstName] = React.useState(user.first_name ?? "");
    const [lastName, setLastName] = React.useState(user.last_name ?? "");
    
    // Glassmorphism theme state
    const theme = useAppStore((state) => state.theme);
    const glassBackground = useAppStore((state) => state.glassBackground);
    const setGlassBackground = useAppStore((state) => state.setGlassBackground);
    const glassBlurLevel = useAppStore((state) => state.glassBlurLevel);
    const setGlassBlurLevel = useAppStore((state) => state.setGlassBlurLevel);
    const glassTransparency = useAppStore((state) => state.glassTransparency);
    const setGlassTransparency = useAppStore((state) => state.setGlassTransparency);

    const uploadAvatar = useUploadUserAvatar();
    const removeAvatar = useDeleteUserAvatar();
    const updateProfile = useUpdateProfile();

    // Auto-save names ~700ms after the user stops typing. A name the server
    // would refuse is not sent; the field shows why and stays unsaved.
    // Memoized so the debounce timer only resets on an actual name change.
    const value = React.useMemo(
        () => ({ firstName: normalizeName(firstName), lastName: normalizeName(lastName) }),
        [firstName, lastName],
    );
    const firstError = nameError("First name", firstName, "person");
    const lastError = nameError("Last name", lastName, "person");
    const autosave = useAutosave({
        value,
        debounceMs: 700,
        save: async (v) => {
            const invalid = nameError("First name", v.firstName, "person") ?? nameError("Last name", v.lastName, "person");
            if (invalid) throw new Error(invalid);
            await updateProfile.mutateAsync({ first_name: v.firstName, last_name: v.lastName });
        },
    });
    useRegisterUnsaved(autosave, () => {
        setFirstName(autosave.savedValue.firstName);
        setLastName(autosave.savedValue.lastName);
    });

    React.useEffect(() => {
        autosave.markSaved({ firstName: user.first_name ?? "", lastName: user.last_name ?? "" });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [user.first_name, user.last_name]);

    // Undo-send window. Same autosave idiom as the names; the mutation
    // patches the cached /auth/me user so the value sticks immediately.
    // 0/undefined means the server cache hasn't caught up yet: show 30.
    const [undoSeconds, setUndoSeconds] = React.useState(user.undo_send_seconds || 30);
    const updateSendPrefs = useUpdateSendPreferences();
    const undoAutosave = useAutosave({
        value: undoSeconds,
        debounceMs: 700,
        save: async (v) => {
            await updateSendPrefs.mutateAsync(Math.min(120, Math.max(5, v)));
        },
    });
    useRegisterUnsaved(undoAutosave, () => {
        setUndoSeconds(undoAutosave.savedValue);
    });

    const status = combineStatus(autosave.status, undoAutosave.status);
    const retry = () => {
        autosave.retry();
        undoAutosave.retry();
    };

    return (
        <SectionShell
            title="Profile"
            description="Used in emails sent on your behalf, the sidebar avatar, and any invitation you send out, plus how the dashboard looks to you."
            actions={<SaveStatus status={status} onRetry={retry} />}
        >
            <Section eyebrow="Identity" description="Names appear on outgoing emails.">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div>
                        <FieldLabel>First name</FieldLabel>
                        <TextInput
                            value={firstName}
                            onChange={setFirstName}
                            invalid={!!firstError}
                            title={firstError ?? undefined}
                            maxLength={PERSON_NAME_MAX}
                            autoComplete="given-name"
                            className="w-full"
                        />
                        <FieldError message={firstError} />
                    </div>
                    <div>
                        <FieldLabel>Last name</FieldLabel>
                        <TextInput
                            value={lastName}
                            onChange={setLastName}
                            invalid={!!lastError}
                            title={lastError ?? undefined}
                            maxLength={PERSON_NAME_MAX}
                            autoComplete="family-name"
                            className="w-full"
                        />
                        <FieldError message={lastError} />
                    </div>
                </div>
                <Row
                    label="Email"
                    description="Email changes go through support for now."
                    align="start"
                >
                    <input
                        type="email"
                        value={user.email}
                        readOnly
                        className="w-full max-w-[280px] h-7 px-2.5 rounded-md border border-slate-200 bg-slate-50 text-[12.5px] text-slate-500 font-mono"
                    />
                </Row>
            </Section>

            <Section
                eyebrow="Appearance"
                description="System follows your device and switches with it. Saved in this browser."
            >
                <ThemeCards />
                
                {/* Glassmorphism customization controls - only shown when Glassmorphism theme is selected */}
                {theme === 'glassmorphism' && (
                    <div className="mt-6 space-y-4">
                        <h3 className="text-sm font-medium text-slate-700">Glassmorphism Settings</h3>
                        
                        <div>
                            <label className="block text-xs font-medium text-slate-700 mb-1">Background Image</label>
                            <div className="grid grid-cols-2 gap-2">
                                <button
                                    type="button"
                                    onClick={() => setGlassBackground('bg1')}
                                    className={`relative rounded-lg overflow-hidden border transition-colors ${glassBackground === 'bg1' ? 'border-sky-500 ring-2 ring-sky-100' : 'border-slate-200 hover:border-slate-300'}`}
                                >
                                    <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-transparent"></div>
                                    <div className="absolute bottom-0 left-0 right-0 p-2 text-white z-10">
                                        <h4 className="text-xs font-medium">Background 1</h4>
                                    </div>
                                    <img 
                                        src="/backdrops/bg1.png" 
                                        alt="Background 1" 
                                        className="w-full h-16 object-cover opacity-80"
                                    />
                                </button>
                                <button
                                    type="button"
                                    onClick={() => setGlassBackground('bg2')}
                                    className={`relative rounded-lg overflow-hidden border transition-colors ${glassBackground === 'bg2' ? 'border-sky-500 ring-2 ring-sky-100' : 'border-slate-200 hover:border-slate-300'}`}
                                >
                                    <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-transparent"></div>
                                    <div className="absolute bottom-0 left-0 right-0 p-2 text-white z-10">
                                        <h4 className="text-xs font-medium">Background 2</h4>
                                    </div>
                                    <img 
                                        src="/backdrops/bg2.png" 
                                        alt="Background 2" 
                                        className="w-full h-16 object-cover opacity-80"
                                    />
                                </button>
                            </div>
                        </div>
                        
                        <div>
                            <label className="block text-xs font-medium text-slate-700 mb-1">Backdrop Blur Level</label>
                            <input
                                type="range"
                                min="8"
                                max="24"
                                value={glassBlurLevel}
                                onChange={(e) => setGlassBlurLevel(parseInt(e.target.value))}
                                className="w-full h-1.5 bg-slate-200 rounded-lg appearance-none cursor-pointer accent-sky-600"
                            />
                            <div className="flex justify-between text-xs text-slate-500 mt-1">
                                <span>Subtle</span>
                                <span>Strong</span>
                            </div>
                        </div>
                        
                        <div>
                            <label className="block text-xs font-medium text-slate-700 mb-1">Transparency Level</label>
                            <input
                                type="range"
                                min="40"
                                max="90"
                                value={glassTransparency}
                                onChange={(e) => setGlassTransparency(parseInt(e.target.value))}
                                className="w-full h-1.5 bg-slate-200 rounded-lg appearance-none cursor-pointer accent-sky-600"
                            />
                            <div className="flex justify-between text-xs text-slate-500 mt-1">
                                <span>Opaque</span>
                                <span>Transparent</span>
                            </div>
                        </div>
                    </div>
                )}
            </Section>

            <TimezonesSection />

            <Section
                eyebrow="Undo send"
                description="Outgoing emails wait this long before sending so you can cancel them from the top bar."
            >
                <Row
                    label="Undo window"
                    description="Between 5 and 120 seconds. Applies to instant sends from the composer and replies."
                >
                    <NumberInput
                        value={undoSeconds}
                        onChange={setUndoSeconds}
                        min={5}
                        max={120}
                        step={5}
                        suffix="seconds"
                        className="w-[150px]"
                    />
                </Row>
            </Section>

            <Section
                eyebrow="Avatar"
                description="Shown in the sidebar, on email previews, and next to your activity. Resized to 512px before upload."
            >
                <AvatarUploader
                    current={user.avatar_url}
                    fallbackInitials={initials(user.email)}
                    shape="circle"
                    onUpload={async (blob) => {
                        await uploadAvatar.mutateAsync(blob);
                    }}
                    onRemove={async () => {
                        await removeAvatar.mutateAsync();
                    }}
                />
            </Section>
        </SectionShell>
    );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
    return (
        <div className="text-[10.5px] uppercase tracking-[0.12em] text-slate-400 font-medium mb-1">
            {children}
        </div>
    );
}
