import type { StateCreator } from 'zustand'

export type Theme = 'light' | 'dark' | 'system'
export type BackgroundPreset = 'default' | 'gradient-1' | 'gradient-2' | 'gradient-3'

export interface AppearanceState {
  glassmorphismEnabled: boolean
  glassOpacity: number
  glassBlur: number
  backgroundPreset: BackgroundPreset
  backgroundImage: string
  backgroundBlur: number
  backgroundOpacity: number
}

export interface UISlice {
  // Sidebar
  sidebarCollapsed: boolean
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
  sidebarCollapsed: false,
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

  // Actions - Sidebar
  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  setSidebarCollapsed: (sidebarCollapsed) =>
    set((state) => (state.sidebarCollapsed === sidebarCollapsed ? state : { sidebarCollapsed })),
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
})
