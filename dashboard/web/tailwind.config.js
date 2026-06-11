// Copyright (c) 2015-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'system-ui', 'sans-serif'],
      },
      // Colors resolve through CSS custom properties so themes can be
      // swapped at runtime. Each token is stored as bare HSL channels
      // ("H S% L%") and consumed with the <alpha-value> placeholder, which
      // is what keeps Tailwind opacity modifiers (e.g. border-border/50)
      // working. Defaults live in src/index.css :root (the Pulse theme).
      colors: {
        primary: {
          DEFAULT: 'hsl(var(--primary) / <alpha-value>)',
          foreground: 'hsl(var(--primary-foreground) / <alpha-value>)',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary) / <alpha-value>)',
          foreground: 'hsl(var(--secondary-foreground) / <alpha-value>)',
        },
        success: {
          DEFAULT: 'hsl(var(--success) / <alpha-value>)',
          foreground: 'hsl(var(--success-foreground) / <alpha-value>)',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive) / <alpha-value>)',
          foreground: 'hsl(var(--destructive-foreground) / <alpha-value>)',
        },
        warning: {
          DEFAULT: 'hsl(var(--warning) / <alpha-value>)',
          foreground: 'hsl(var(--warning-foreground) / <alpha-value>)',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted) / <alpha-value>)',
          foreground: 'hsl(var(--muted-foreground) / <alpha-value>)',
        },
        background: 'hsl(var(--background) / <alpha-value>)',
        foreground: 'hsl(var(--foreground) / <alpha-value>)',
        card: {
          DEFAULT: 'hsl(var(--card) / <alpha-value>)',
          foreground: 'hsl(var(--card-foreground) / <alpha-value>)',
        },
        border: 'hsl(var(--border) / <alpha-value>)',
      },
      // Gradient channels are themeable; the per-stop alpha stays baked into
      // the gradient string (backgroundImage does not take <alpha-value>).
      backgroundImage: {
        'gradient-primary': 'linear-gradient(135deg, hsl(var(--gradient-primary-from)) 0%, hsl(var(--gradient-primary-to)) 100%)',
        'gradient-card': 'linear-gradient(135deg, hsl(var(--gradient-card-from) / 0.5) 0%, hsl(var(--gradient-card-to) / 0.3) 100%)',
      },
      keyframes: {
        'fade-in': {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        'spin': {
          'from': { transform: 'rotate(0deg)' },
          'to': { transform: 'rotate(360deg)' }
        },
        'flash-buy': {
          '0%': { backgroundColor: 'hsl(var(--success) / 0.5)' },
          '100%': { backgroundColor: 'hsl(var(--success) / 0)' }
        },
        'flash-sell': {
          '0%': { backgroundColor: 'hsl(var(--destructive) / 0.5)' },
          '100%': { backgroundColor: 'hsl(var(--destructive) / 0)' }
        }
      },
      animation: {
        'fade-in': 'fade-in 0.5s ease-out',
        'spin': 'spin 1s linear infinite',
        'flash-buy': 'flash-buy 0.7s ease-out',
        'flash-sell': 'flash-sell 0.7s ease-out',
      },
    },
  },
  plugins: [],
}
