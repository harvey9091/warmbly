import type { StateCreator } from 'zustand'

export type Theme = 'light' | 'dark' | 'system'
export type BackgroundPreset = 'default' | 'gradient-1' | 'gradient-4' | 'gradient-5'

export interface AppearanceState {
  glassmorphismEnabled: boolean
  glassOpacity: number
  glassBlur: number
  backgroundPreset: BackgroundPreset
  backgroundImage: string
  backgroundBlur: number
  backgroundOpacity: number
}

// Unibox list-column bounds. These are the preference's bounds; what the column
// can actually render is additionally capped against the viewport at the drag
// site, so the stored value survives a narrow window instead of being rewritten
// by it.
export const UNIBOX_LIST_MIN_WIDTH = 280
export const UNIBOX_LIST_MAX_WIDTH = 620
export const UNIBOX_LIST_DEFAULT_WIDTH = 360

// Exported because rehydration bypasses the setter: zustand's default merge
// writes localStorage straight into state, so the clamp has to run there too or
// a hand-edited (or newly out-of-range) value reaches the DOM unchecked.
export const clampUniboxListWidth = (w: unknown): number => {
  // Only a real number survives. Coercing would be worse than useless here:
  // `Number(null)` is 0, so a null in storage would silently become the minimum
  // width instead of falling back to the default.
  if (typeof w !== 'number' || !Number.isFinite(w)) return UNIBOX_LIST_DEFAULT_WIDTH
  return Math.round(Math.min(UNIBOX_LIST_MAX_WIDTH, Math.max(UNIBOX_LIST_MIN_WIDTH, w)))
}

export interface UISlice {
  // Sidebar. Deliberately NOT the old `sidebarCollapsed` key: that one was
  // persisted and toggled by `b` for a long time while nothing rendered from
  // it, so a stored `true` reflects a keystroke nobody remembers. A new key
  // starts everyone expanded without needing a migration, which zustand would
  // not have run anyway: it only migrates a store whose version is a number,
  // and every store written before this had no version field at all.
  navCollapsed: boolean
  sidebarMobileOpen: boolean

  // Theme
  theme: Theme
  resolvedTheme: 'light' | 'dark'

  // Appearance
  glassmorphismEnabled: boolean
  glassOpacity: number
  glassBlur: number
  backgroundPreset: BackgroundPreset
  backgroundImage: string
  backgroundBlur: number
  backgroundOpacity: number

  // Modals
  tagsModalOpen: boolean
  foldersModalOpen: boolean
  addEmailModalOpen: boolean
  shortcutsModalOpen: boolean
  commandPaletteOpen: boolean

  // AI assistant panel (right-side, persistent across routes)
  aiAssistantOpen: boolean

  // Unibox layout preferences (persisted). The list column is drag-resizable
  // against the thread pane; the CRM rail remembers the last explicit toggle
  // so closing it survives opening the next thread.
  uniboxListWidth: number
  uniboxContactRailOpen: boolean

  // Actions - Sidebar
  toggleSidebar: () => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setSidebarMobileOpen: (open: boolean) => void

  // Actions - Theme
  setTheme: (theme: Theme) => void
  setResolvedTheme: (theme: 'light' | 'dark') => void

  // Actions - Appearance
  setGlassmorphismEnabled: (enabled: boolean) => void
  setGlassOpacity: (opacity: number) => void
  setGlassBlur: (blur: number) => void
  setBackgroundPreset: (preset: BackgroundPreset) => void
  setBackgroundImage: (url: string) => void
  setBackgroundBlur: (blur: number) => void
  setBackgroundOpacity: (opacity: number) => void

  // Actions - Modals
  setTagsModalOpen: (tagsModalOpen: boolean) => void
  setFoldersModalOpen: (foldersModalOpen: boolean) => void
  setAddEmailModalOpen: (addEmailModalOpen: boolean) => void
  setShortcutsModalOpen: (shortcutsModalOpen: boolean) => void
  setCommandPaletteOpen: (commandPaletteOpen: boolean) => void
  setAIAssistantOpen: (aiAssistantOpen: boolean) => void
  toggleAIAssistant: () => void

  // Actions - Unibox layout
  setUniboxListWidth: (width: number) => void
  setUniboxContactRailOpen: (open: boolean) => void
}

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') return 'system'
  return (localStorage.getItem('theme') as Theme) || 'system'
}

const getResolvedTheme = (_theme: Theme): 'light' | 'dark' => {
  if (_theme === 'system') {
    if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark'
    }
    return 'light'
  }
  return _theme
}

const getInitialAppearance = (): AppearanceState => {
  if (typeof window === 'undefined') {
    return {
      glassmorphismEnabled: false,
      glassOpacity: 88,
      glassBlur: 12,
      backgroundPreset: 'default',
      backgroundImage: '',
      backgroundBlur: 0,
      backgroundOpacity: 100,
    }
  }
  try {
    const raw = localStorage.getItem('warmbly-appearance')
    if (raw) {
      const parsed = JSON.parse(raw) as AppearanceState
      return {
        glassmorphismEnabled: parsed.glassmorphismEnabled ?? false,
        glassOpacity: Math.max(60, parsed.glassOpacity ?? 88),
        glassBlur: parsed.glassBlur ?? 12,
        backgroundPreset: parsed.backgroundPreset ?? 'default',
        backgroundImage: parsed.backgroundImage ?? '',
        backgroundBlur: parsed.backgroundBlur ?? 0,
        backgroundOpacity: parsed.backgroundOpacity ?? 100,
      }
    }
  } catch { /* ignore */ }
  return {
    glassmorphismEnabled: false,
    glassOpacity: 88,
    glassBlur: 12,
    backgroundPreset: 'default',
    backgroundImage: '',
    backgroundBlur: 0,
    backgroundOpacity: 100,
  }
}

const saveAppearance = (state: Pick<AppearanceState, 'glassmorphismEnabled' | 'glassOpacity' | 'glassBlur' | 'backgroundPreset' | 'backgroundImage' | 'backgroundBlur' | 'backgroundOpacity'>) => {
  if (typeof window === 'undefined') return
  localStorage.setItem('warmbly-appearance', JSON.stringify(state))
}

export const createUISlice: StateCreator<UISlice, [], [], UISlice> = (set, get) => ({
  // Sidebar
  navCollapsed: false,
  sidebarMobileOpen: false,

  // Theme
  theme: getInitialTheme(),
  resolvedTheme: getResolvedTheme(getInitialTheme()),

  // Appearance — initialized from localStorage; falls back to defaults.
  ...getInitialAppearance(),

  // Modals
  tagsModalOpen: false,
  foldersModalOpen: false,
  addEmailModalOpen: false,
  shortcutsModalOpen: false,
  commandPaletteOpen: false,
  aiAssistantOpen: false,

  // Unibox layout
  uniboxListWidth: UNIBOX_LIST_DEFAULT_WIDTH,
  uniboxContactRailOpen: true,

  // Actions - Sidebar
  toggleSidebar: () => set((state) => ({ navCollapsed: !state.navCollapsed })),
  setSidebarCollapsed: (navCollapsed) =>
    set((state) => (state.navCollapsed === navCollapsed ? state : { navCollapsed })),
  setSidebarMobileOpen: (sidebarMobileOpen) =>
    set((state) => (state.sidebarMobileOpen === sidebarMobileOpen ? state : { sidebarMobileOpen })),

  // Actions - Theme
  setTheme: (theme) => {
    if (get().theme === theme) return
    localStorage.setItem('theme', theme)
    const resolvedTheme = getResolvedTheme(theme)
    if (resolvedTheme === 'dark') {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
    set({ theme, resolvedTheme })
    if (resolvedTheme === 'dark') {
      const current = get()
      if (!current.backgroundImage) {
        current.setBackgroundImage('/backgrounds/bg-1.png')
        current.setBackgroundPreset('default')
      }
    }
  },
  setResolvedTheme: (resolvedTheme) =>
    set((state) => (state.resolvedTheme === resolvedTheme ? state : { resolvedTheme })),

  // Actions - Appearance
  setGlassmorphismEnabled: (glassmorphismEnabled) =>
    set((state) => {
      if (state.glassmorphismEnabled === glassmorphismEnabled) return state
      const next = { ...state, glassmorphismEnabled }
      saveAppearance(next)
      return next
    }),
  setGlassOpacity: (glassOpacity) =>
    set((state) => {
      const next = Math.max(60, Math.min(100, glassOpacity))
      if (state.glassOpacity === next) return state
      const upd = { ...state, glassOpacity: next }
      saveAppearance(upd)
      return upd
    }),
  setGlassBlur: (glassBlur) =>
    set((state) => {
      const next = { ...state, glassBlur }
      saveAppearance(next)
      return next
    }),
  setBackgroundPreset: (backgroundPreset) =>
    set((state) => {
      const next = { ...state, backgroundPreset }
      saveAppearance(next)
      return next
    }),
  setBackgroundImage: (backgroundImage) =>
    set((state) => {
      const next = { ...state, backgroundImage }
      saveAppearance(next)
      return next
    }),
  setBackgroundBlur: (backgroundBlur) =>
    set((state) => {
      const next = { ...state, backgroundBlur }
      saveAppearance(next)
      return next
    }),
  setBackgroundOpacity: (backgroundOpacity) =>
    set((state) => {
      const next = { ...state, backgroundOpacity }
      saveAppearance(next)
      return next
    }),

  // Actions - Modals
  setTagsModalOpen: (tagsModalOpen) =>
    set((state) => (state.tagsModalOpen === tagsModalOpen ? state : { tagsModalOpen })),
  setFoldersModalOpen: (foldersModalOpen) =>
    set((state) => (state.foldersModalOpen === foldersModalOpen ? state : { foldersModalOpen })),
  setAddEmailModalOpen: (addEmailModalOpen) =>
    set((state) => (state.addEmailModalOpen === addEmailModalOpen ? state : { addEmailModalOpen })),
  setShortcutsModalOpen: (shortcutsModalOpen) =>
    set((state) => (state.shortcutsModalOpen === shortcutsModalOpen ? state : { shortcutsModalOpen })),
  setCommandPaletteOpen: (commandPaletteOpen) =>
    set((state) => (state.commandPaletteOpen === commandPaletteOpen ? state : { commandPaletteOpen })),
  setAIAssistantOpen: (aiAssistantOpen) =>
    set((state) => (state.aiAssistantOpen === aiAssistantOpen ? state : { aiAssistantOpen })),
  toggleAIAssistant: () => set((state) => ({ aiAssistantOpen: !state.aiAssistantOpen })),

  // Actions - Unibox layout
  setUniboxListWidth: (width) => {
    const uniboxListWidth = clampUniboxListWidth(width)
    set((state) => (state.uniboxListWidth === uniboxListWidth ? state : { uniboxListWidth }))
  },
  setUniboxContactRailOpen: (uniboxContactRailOpen) =>
    set((state) =>
      state.uniboxContactRailOpen === uniboxContactRailOpen ? state : { uniboxContactRailOpen },
    ),
})
