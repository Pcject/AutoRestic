import React from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider as MuiThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import { AuthProvider, useAuth } from './AuthContext.jsx';
import { ThemeProvider, useTheme } from './ThemeContext.jsx';
import { lightTheme, darkTheme } from './theme/theme';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import Repositories from './pages/Repositories';
import Backups from './pages/Backups';
import Schedules from './pages/Schedules';
import Tasks from './pages/Tasks';
import Logs from './pages/Logs';
import Layout from './components/Layout';

const ProtectedRoute = ({ children }) => {
  const { user, loading } = useAuth();
  
  if (loading) {
    return <div>Loading...</div>;
  }
  
  if (!user) {
    return <Navigate to="/login" />;
  }
  
  return children;
};

const PublicRoute = ({ children }) => {
  const { user, loading } = useAuth();
  
  if (loading) {
    return <div>Loading...</div>;
  }
  
  if (user) {
    return <Navigate to="/dashboard" />;
  }
  
  return children;
};

const ThemedApp = () => {
  const { mode } = useTheme();
  const theme = mode === 'light' ? lightTheme : darkTheme;

  return (
    <MuiThemeProvider theme={theme}>
      <CssBaseline />
      <AuthProvider>
        <Router>
          <Routes>
            <Route path="/login" element={
              <PublicRoute>
                <Login />
              </PublicRoute>
            } />
            <Route path="/" element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }>
              <Route index element={<Navigate to="/dashboard" />} />
              <Route path="dashboard" element={<Dashboard />} />
              <Route path="repositories" element={<Repositories />} />
              <Route path="backups" element={<Backups />} />
              <Route path="schedules" element={<Schedules />} />
              <Route path="tasks" element={<Tasks />} />
              <Route path="logs" element={<Logs />} />
            </Route>
          </Routes>
        </Router>
      </AuthProvider>
    </MuiThemeProvider>
  );
};

function App() {
  return (
    <ThemeProvider>
      <ThemedApp />
    </ThemeProvider>
  );
}

export default App;
