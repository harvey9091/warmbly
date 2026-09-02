/**
 * Grain gradient palettes for the product frames.
 *
 * One blue family, held at a few close tints so the grain reads as light over
 * a surface rather than as colour. The variants exist to CONTRAST with the
 * band behind them, not to match it: a frame the same colour as its section
 * disappears. Dark bands get the light variant, light and bright bands get
 * the deeper ones, and on a whiteish band the frame stays blue.
 *
 * `frame` seeds the animation, so two frames sharing a palette start apart.
 */
export interface GrainToneSpec {
  back: string;
  /** Up to 7; one hue only. */
  colors: string[];
  frame: number;
  /** CSS gradient painted underneath, for no-WebGL. */
  css: string;
}

export const GRAIN_TONES = {
  /** Light blue. For the dark bands (navy, wallpaper). */
  light: {
    back: '#4aa6d8',
    colors: ['#66b9e6', '#87cdf1', '#a9def8', '#cdeefd'],
    frame: 1200,
    css: 'radial-gradient(90% 80% at 20% 0%, #a9def8 0%, rgba(169,222,248,0) 62%), linear-gradient(160deg, #87cdf1 0%, #5fb4e3 50%, #4aa6d8 100%)',
  },
  /** Mid blue. The default, and what sits on a whiteish band. */
  mid: {
    back: '#2f8ac6',
    colors: ['#4aa1da', '#6cb9e9', '#8fcef4', '#b6e3fb'],
    frame: 4800,
    css: 'radial-gradient(85% 75% at 80% 4%, #8fcef4 0%, rgba(143,206,244,0) 58%), linear-gradient(165deg, #6cb9e9 0%, #4098d2 52%, #2f8ac6 100%)',
  },
  /** Deep blue. For the bright sky band, which the mid tone would blend into. */
  deep: {
    back: '#12456f',
    colors: ['#1c5c92', '#2a76b2', '#3c90cd', '#55a9e0'],
    frame: 9100,
    css: 'radial-gradient(80% 70% at 76% 4%, #3c90cd 0%, rgba(60,144,205,0) 60%), linear-gradient(168deg, #2a76b2 0%, #17527f 55%, #12456f 100%)',
  },
} as const satisfies Record<string, GrainToneSpec>;

export type GrainTone = keyof typeof GRAIN_TONES;
