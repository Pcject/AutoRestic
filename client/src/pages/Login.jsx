import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Container,
  Box,
  TextField,
  Button,
  Typography,
  Paper,
  Alert,
  Avatar,
  IconButton,
  InputAdornment,
  alpha,
} from '@mui/material';
import {
  Backup as BackupIcon,
  Visibility as VisibilityIcon,
  VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
import { useAuth } from '../AuthContext';

const Login = () => {
  const navigate = useNavigate();
  const { login } = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [showPassword, setShowPassword] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');

    try {
      await login(username, password);
      navigate('/dashboard');
    } catch (err) {
      setError(err.response?.data?.error || '登录失败');
    }
  };

  return (
    <Container component="main" maxWidth="sm">
      <Box
        sx={{
          minHeight: '100vh',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          py: 4,
        }}
      >
        <Paper
          elevation={0}
          sx={{
            p: { xs: 3, sm: 5 },
            width: '100%',
            borderRadius: 4,
            boxShadow: (theme) =>
              theme.palette.mode === 'light'
                ? '0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)'
                : '0 20px 25px -5px rgba(0, 0, 0, 0.3), 0 10px 10px -5px rgba(0, 0, 0, 0.2)',
            background: (theme) =>
              theme.palette.mode === 'light'
                ? 'linear-gradient(145deg, #ffffff 0%, #f8fafc 100%)'
                : 'linear-gradient(145deg, #1e293b 0%, #0f172a 100%)',
          }}
        >
          <Box sx={{ textAlign: 'center', mb: 4 }}>
            <Avatar
              sx={{
                width: 80,
                height: 80,
                mx: 'auto',
                mb: 2,
                background: (theme) =>
                  `linear-gradient(135deg, ${theme.palette.primary.main} 0%, ${theme.palette.primary.dark} 100%)`,
                boxShadow: (theme) =>
                  `0 10px 25px -5px ${alpha(theme.palette.primary.main, 0.4)}, 0 4px 6px -2px ${alpha(theme.palette.primary.main, 0.2)}`,
              }}
            >
              <BackupIcon sx={{ fontSize: 40 }} />
            </Avatar>
            <Typography
              component="h1"
              variant="h3"
              sx={{
                fontWeight: 700,
                background: (theme) =>
                  `linear-gradient(135deg, ${theme.palette.primary.main} 0%, ${theme.palette.secondary.main} 100%)`,
                backgroundClip: 'text',
                WebkitBackgroundClip: 'text',
                WebkitTextFillColor: 'transparent',
              }}
            >
              AutoRestic
            </Typography>
            <Typography
              component="p"
              variant="body1"
              color="text.secondary"
              sx={{ mt: 1, fontWeight: 500 }}
            >
              基于 Restic 的自动备份服务
            </Typography>
          </Box>

          <Box component="form" onSubmit={handleSubmit}>
            {error && (
              <Alert
                severity="error"
                sx={{
                  mb: 3,
                  borderRadius: 2,
                }}
              >
                {error}
              </Alert>
            )}

            <TextField
              margin="normal"
              required
              fullWidth
              id="username"
              label="用户名"
              name="username"
              autoComplete="username"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              variant="outlined"
              sx={{
                mb: 2,
                '& .MuiOutlinedInput-root': {
                  transition: 'all 0.2s',
                },
              }}
            />

            <TextField
              margin="normal"
              required
              fullWidth
              name="password"
              label="密码"
              type={showPassword ? 'text' : 'password'}
              id="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              variant="outlined"
              sx={{
                mb: 3,
                '& .MuiOutlinedInput-root': {
                  transition: 'all 0.2s',
                },
              }}
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label="toggle password visibility"
                      onClick={() => setShowPassword(!showPassword)}
                      edge="end"
                    >
                      {showPassword ? <VisibilityOffIcon /> : <VisibilityIcon />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />

            <Button
              type="submit"
              fullWidth
              variant="contained"
              size="large"
              sx={{
                py: 1.8,
                fontWeight: 600,
                fontSize: '1rem',
                borderRadius: 2,
                textTransform: 'none',
                background: (theme) =>
                  `linear-gradient(135deg, ${theme.palette.primary.main} 0%, ${theme.palette.primary.dark} 100%)`,
                boxShadow: (theme) =>
                  `0 4px 6px -1px ${alpha(theme.palette.primary.main, 0.3)}, 0 2px 4px -1px ${alpha(theme.palette.primary.main, 0.2)}`,
                '&:hover': {
                  background: (theme) =>
                    `linear-gradient(135deg, ${theme.palette.primary.light} 0%, ${theme.palette.primary.main} 100%)`,
                  boxShadow: (theme) =>
                    `0 10px 15px -3px ${alpha(theme.palette.primary.main, 0.4)}, 0 4px 6px -2px ${alpha(theme.palette.primary.main, 0.2)}`,
                },
              }}
            >
              登录
            </Button>
          </Box>
        </Paper>

        <Typography
          variant="body2"
          color="text.secondary"
          sx={{ mt: 4, textAlign: 'center' }}
        >
          安全、可靠的备份解决方案
        </Typography>
      </Box>
    </Container>
  );
};

export default Login;
