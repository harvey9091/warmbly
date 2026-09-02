import { useEffect } from 'react'
import { useAppStore } from '@/stores'

// Mirrors the stored theme preference onto the document.documentElement `.dark`
// class so the CSS variable system in global.css can resolve the correct palette.
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const theme = useAppStore((state) => state.theme)
  const setResolvedTheme = useAppStore((state) => state.setResolvedTheme)

  useEffect(() => {
    const resolved = theme === 'system'
      ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : theme

    if (resolved === 'dark') {
      document.documentElement.classList.add('dark')
    } else {
      document.documentElement.classList.remove('dark')
    }
    setResolvedTheme(resolved)
  }, [theme, setResolvedTheme])

  return <>{children}</>
}
