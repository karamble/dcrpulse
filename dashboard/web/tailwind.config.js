// Copyright (c) 2015-2025 The Decred developers
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
      colors: {
        primary: {
          DEFAULT: 'hsl(217 91% 60%)',
          foreground: 'hsl(222 47% 5%)',
        },
        secondary: {
          DEFAULT: 'hsl(173 80% 40%)',
          foreground: 'hsl(210 40% 98%)',
        },
        success: {
          DEFAULT: 'hsl(142 76% 36%)',
          foreground: 'hsl(210 40% 98%)',
        },
        warning: {
          DEFAULT: 'hsl(38 92% 50%)',
          foreground: 'hsl(222 47% 5%)',
        },
        muted: {
          DEFAULT: 'hsl(217 32% 17%)',
          foreground: 'hsl(215 20% 65%)',
        },
        background: 'hsl(222 47% 5%)',
        foreground: 'hsl(210 40% 98%)',
        card: {
          DEFAULT: 'hsl(222 47% 8%)',
          foreground: 'hsl(210 40% 98%)',
        },
        border: 'hsl(217 32% 17%)',
      },
      backgroundImage: {
        'gradient-primary': 'linear-gradient(135deg, hsl(217 91% 60%) 0%, hsl(173 80% 40%) 100%)',
        'gradient-card': 'linear-gradient(135deg, hsl(222 47% 8% / 0.5) 0%, hsl(222 47% 10% / 0.3) 100%)',
      },
      keyframes: {
        'fade-in': {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        'spin': {
          'from': { transform: 'rotate(0deg)' },
          'to': { transform: 'rotate(360deg)' }
        }
      },
      animation: {
        'fade-in': 'fade-in 0.5s ease-out',
        'spin': 'spin 1s linear infinite',
      },
    },
  },
  plugins: [],
}

