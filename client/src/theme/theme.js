import { createTheme } from '@mui/material/styles';

const getDesignTokens = (mode) => ({
  palette: {
    mode,
    ...(mode === 'light'
      ? {
          primary: {
            main: '#5C6BC0',
            light: '#7986CB',
            dark: '#3949AB',
          },
          secondary: {
            main: '#EC407A',
            light: '#F48FB1',
            dark: '#C2185B',
          },
          background: {
            default: '#F5F7FA',
            paper: '#FFFFFF',
          },
          text: {
            primary: '#1A202C',
            secondary: '#4A5568',
          },
          success: {
            main: '#10B981',
          },
          warning: {
            main: '#F59E0B',
          },
          error: {
            main: '#EF4444',
          },
          info: {
            main: '#3B82F6',
          },
        }
      : {
          primary: {
            main: '#818CF8',
            light: '#A5B4FC',
            dark: '#6366F1',
          },
          secondary: {
            main: '#F472B6',
            light: '#F9A8D4',
            dark: '#EC4899',
          },
          background: {
            default: '#0F172A',
            paper: '#1E293B',
          },
          text: {
            primary: '#F1F5F9',
            secondary: '#94A3B8',
          },
          success: {
            main: '#34D399',
          },
          warning: {
            main: '#FBBF24',
          },
          error: {
            main: '#F87171',
          },
          info: {
            main: '#60A5FA',
          },
        }),
  },
  typography: {
    fontFamily: '"Inter", "Roboto", "Helvetica", "Arial", sans-serif',
    h1: {
      fontWeight: 700,
    },
    h2: {
      fontWeight: 600,
    },
    h3: {
      fontWeight: 600,
    },
    h4: {
      fontWeight: 600,
    },
    h5: {
      fontWeight: 500,
    },
    h6: {
      fontWeight: 500,
    },
  },
  shape: {
    borderRadius: 12,
  },
  shadows: [
    'none',
    '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
    '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
    '0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)',
    '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)',
    ...Array.from({ length: 20 }).map(
      (_, i) =>
        `0 ${i + 5}px ${i + 10}px -${i + 3}px rgba(0, 0, 0, 0.1), 0 ${
          i + 2
        }px ${i + 4}px -${i + 2}px rgba(0, 0, 0, 0.05)`
    ),
  ],
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          textTransform: 'none',
          borderRadius: 8,
          fontWeight: 500,
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 16,
          boxShadow:
            '0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)',
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          borderRadius: 12,
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            borderRadius: 8,
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 6,
        },
      },
    },
    MuiTable: {
      styleOverrides: {
        root: {
          borderRadius: 12,
          overflow: 'hidden',
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          '&:hover': {
            backgroundColor: 'rgba(92, 107, 192, 0.04)',
          },
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          boxShadow:
            '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
        },
      },
    },
  },
});

export const lightTheme = createTheme(getDesignTokens('light'));
export const darkTheme = createTheme(getDesignTokens('dark'));
