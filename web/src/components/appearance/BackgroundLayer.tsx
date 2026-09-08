// BackgroundLayer — a fixed, pointer-events-none layer behind the entire
// application that displays the user-selected background image and
// responds to blur/opacity preferences stored in the appearance state.
//
// Sits at z-index: 0 behind the entire app shell. Inline styles drive the
// visual values so transitions fire when the user changes settings.
// Toggles the `appearance-bg-active` class on <html> so other surfaces
// can adjust their own opacity when a background is present.

import { useEffect } from 'react'
import { useAppStore } from '@/stores'

const PRESET_GRADIENTS: Record<string, string> = {
  'gradient-1': 'linear-gradient(170deg, #f8fafc 0%, #e0f2fe 18%, #fef3c7 48%, #fde68a 78%, #fefce8 100%)',
  'gradient-2': 'linear-gradient(165deg, #faf5ff 0%, #f3e8ff 22%, #fce7f3 52%, #fbcfe8 82%, #fdf2f8 100%)',
  'gradient-3': 'linear-gradient(150deg, #f0fdf4 0%, #d1fae5 22%, #a7f3d0 52%, #e0f2fe 82%, #f0f9ff 100%)',
}

export function BackgroundLayer() {
  const backgroundPreset = useAppStore((state) => state.backgroundPreset)
  const backgroundImage = useAppStore((state) => state.backgroundImage)
  const backgroundBlur = useAppStore((state) => state.backgroundBlur)
  const backgroundOpacity = useAppStore((state) => state.backgroundOpacity)

  const hasImage = backgroundImage || backgroundPreset !== 'default'

  useEffect(() => {
    const root = document.documentElement
    if (!hasImage) {
      root.classList.remove('appearance-bg-active')
      return
    }
    root.classList.add('appearance-bg-active')
  }, [hasImage])

  const layerStyle: React.CSSProperties = {}
  if (backgroundImage) {
    layerStyle.backgroundImage = `url("${backgroundImage}")`
    layerStyle.backgroundSize = 'cover'
    layerStyle.backgroundPosition = 'center'
    layerStyle.backgroundRepeat = 'no-repeat'
  } else if (backgroundPreset !== 'default' && PRESET_GRADIENTS[backgroundPreset]) {
    layerStyle.background = PRESET_GRADIENTS[backgroundPreset]
  }

  if (hasImage) {
    layerStyle.opacity = backgroundOpacity / 100
    layerStyle.filter = backgroundBlur ? `blur(${backgroundBlur}px)` : undefined
  } else {
    layerStyle.opacity = 1
    layerStyle.filter = undefined
  }

  if (!hasImage) return null

  return <div className="appearance-bg-layer" style={layerStyle} aria-hidden="true" />
}
