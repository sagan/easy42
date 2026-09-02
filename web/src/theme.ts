import { createTheme } from '@mui/material/styles';

export const theme = createTheme({
  palette: {
    mode: 'light',
    background: {
      default: '#F8FAFC',
      paper: '#FFFFFF',
    },
    primary: {
      main: '#4F46E5', // Indigo
      light: '#6366F1',
      dark: '#3730A3',
    },
    secondary: {
      main: '#0891B2', // Cyan
      light: '#06B6D4',
      dark: '#0E7490',
    },
    success: {
      main: '#059669',
      light: '#10B981',
      dark: '#047857',
    },
    warning: {
      main: '#D97706',
      light: '#F59E0B',
      dark: '#B45309',
    },
    error: {
      main: '#E11D48',
      light: '#F43F5E',
      dark: '#BE123C',
    },
    text: {
      primary: '#0F172A',
      secondary: '#64748B',
    },
    divider: '#E2E8F0',
  },
  typography: {
    fontFamily: '"Plus Jakarta Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
    h1: { fontWeight: 700, color: '#0F172A' },
    h2: { fontWeight: 700, color: '#0F172A' },
    h3: { fontWeight: 700, color: '#0F172A' },
    h4: { fontWeight: 600, color: '#0F172A' },
    h5: { fontWeight: 600, color: '#0F172A' },
    h6: { fontWeight: 600, color: '#0F172A' },
    button: { textTransform: 'none', fontWeight: 600 },
  },
  shape: {
    borderRadius: 10,
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          padding: '8px 18px',
          boxShadow: 'none',
          transition: 'all 0.15s ease-in-out',
          '&:hover': {
            boxShadow: '0 4px 12px rgba(79, 70, 229, 0.2)',
            transform: 'translateY(-1px)',
          },
        },
        containedPrimary: {
          backgroundColor: '#4F46E5',
          '&:hover': {
            backgroundColor: '#4338CA',
          },
        },
        containedSecondary: {
          backgroundColor: '#0891B2',
          '&:hover': {
            backgroundColor: '#0E7490',
            boxShadow: '0 4px 12px rgba(8, 145, 178, 0.25)',
          },
        },
        outlined: {
          borderColor: '#E2E8F0',
          color: '#334155',
          '&:hover': {
            borderColor: '#CBD5E1',
            backgroundColor: '#F1F5F9',
          },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: '#FFFFFF',
          border: '1px solid #E2E8F0',
          boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.05), 0 1px 2px -1px rgba(0, 0, 0, 0.05)',
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundColor: '#FFFFFF',
          borderRadius: 16,
          border: '1px solid #E2E8F0',
          boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1)',
        },
      },
    },
    MuiTextField: {
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            backgroundColor: '#FFFFFF',
            borderRadius: 8,
            '& fieldset': {
              borderColor: '#CBD5E1',
            },
            '&:hover fieldset': {
              borderColor: '#94A3B8',
            },
            '&.Mui-focused fieldset': {
              borderColor: '#4F46E5',
              boxShadow: '0 0 0 3px rgba(79, 70, 229, 0.15)',
            },
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          fontWeight: 600,
          borderRadius: 6,
        },
      },
    },
  },
});
