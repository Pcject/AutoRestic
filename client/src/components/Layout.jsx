import React, { useState, useMemo, useEffect } from 'react';
import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom';
import {
  AppBar,
  Box,
  Toolbar,
  Typography,
  IconButton,
  Drawer,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Divider,
  Avatar,
  Tooltip,
  useTheme,
  useMediaQuery,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Alert,
  Snackbar,
} from '@mui/material';
import {
  Menu as MenuIcon,
  Dashboard as DashboardIcon,
  Storage as StorageIcon,
  Backup as BackupIcon,
  Schedule as ScheduleIcon,
  Assignment as TaskIcon,
  Description as DescriptionIcon,
  Logout as LogoutIcon,
  Brightness4 as DarkModeIcon,
  Brightness7 as LightModeIcon,
  Settings as SettingsIcon,
  ChevronLeft as ChevronLeftIcon,
  ChevronRight as ChevronRightIcon,
  Visibility as VisibilityIcon,
  VisibilityOff as VisibilityOffIcon,
} from '@mui/icons-material';
import { useTheme as useCustomTheme } from '../ThemeContext.jsx';
import { authAPI } from '../api';
import { useAuth } from '../AuthContext.jsx';

const drawerWidth = 260;
const drawerWidthCollapsed = 64;

const menuItems = [
  { text: '仪表盘', icon: <DashboardIcon />, path: '/dashboard' },
  { text: '仓库管理', icon: <StorageIcon />, path: '/repositories' },
  { text: '备份管理', icon: <BackupIcon />, path: '/backups' },
  { text: '定时任务', icon: <ScheduleIcon />, path: '/schedules' },
  { text: '任务管理', icon: <TaskIcon />, path: '/tasks' },
  { text: '日志记录', icon: <DescriptionIcon />, path: '/logs' },
];

const ProfileDialog = ({ open, onClose, user, onSuccess }) => {
  const [username, setUsername] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (open && user) {
      setUsername(user.username);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setError('');
    }
  }, [open, user]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError('');

    if (newPassword && newPassword !== confirmPassword) {
      setError('新密码和确认密码不一致');
      return;
    }

    if (newPassword && newPassword.length < 6) {
      setError('新密码至少需要6个字符');
      return;
    }

    setLoading(true);
    try {
      const data = {};
      if (username !== user.username) {
        data.username = username;
      }
      if (newPassword) {
        data.currentPassword = currentPassword;
        data.newPassword = newPassword;
      }

      if (Object.keys(data).length > 0) {
        await authAPI.updateProfile(data);
        onSuccess();
        onClose();
      }
    } catch (err) {
      setError(err.response?.data?.error || '更新失败');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>账户设置</DialogTitle>
      <form onSubmit={handleSubmit}>
        <DialogContent>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          <TextField
            fullWidth
            label="用户名"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            margin="normal"
            required
          />
          <TextField
            fullWidth
            label="当前密码（修改密码时必填）"
            type={showCurrentPassword ? 'text' : 'password'}
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            margin="normal"
            InputProps={{
              endAdornment: (
                <IconButton onClick={() => setShowCurrentPassword(!showCurrentPassword)} edge="end">
                  {showCurrentPassword ? <VisibilityOffIcon /> : <VisibilityIcon />}
                </IconButton>
              ),
            }}
          />
          <TextField
            fullWidth
            label="新密码（留空则不修改）"
            type={showNewPassword ? 'text' : 'password'}
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            margin="normal"
            InputProps={{
              endAdornment: (
                <IconButton onClick={() => setShowNewPassword(!showNewPassword)} edge="end">
                  {showNewPassword ? <VisibilityOffIcon /> : <VisibilityIcon />}
                </IconButton>
              ),
            }}
          />
          <TextField
            fullWidth
            label="确认新密码"
            type={showConfirmPassword ? 'text' : 'password'}
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            margin="normal"
            disabled={!newPassword}
            InputProps={{
              endAdornment: (
                <IconButton 
                  onClick={() => setShowConfirmPassword(!showConfirmPassword)} 
                  edge="end"
                  disabled={!newPassword}
                >
                  {showConfirmPassword ? <VisibilityOffIcon /> : <VisibilityIcon />}
                </IconButton>
              ),
            }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} disabled={loading}>取消</Button>
          <Button type="submit" variant="contained" disabled={loading}>
            {loading ? '保存中...' : '保存'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};

const Layout = () => {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [profileDialogOpen, setProfileDialogOpen] = useState(false);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));
  const navigate = useNavigate();
  const location = useLocation();
  const { mode, toggleMode } = useCustomTheme();
  const { user, refreshUser } = useAuth();

  const handleDrawerToggle = () => {
    if (isMobile) {
      setMobileOpen(!mobileOpen);
    } else {
      setSidebarCollapsed(!sidebarCollapsed);
    }
  };

  const handleLogout = () => {
    localStorage.removeItem('token');
    navigate('/login');
  };

  const handleProfileSuccess = () => {
    refreshUser();
    setSnackbar({ open: true, message: '账户设置更新成功', severity: 'success' });
  };

  const currentDrawerWidth = useMemo(() => {
    if (isMobile) return drawerWidth;
    return sidebarCollapsed ? drawerWidthCollapsed : drawerWidth;
  }, [isMobile, sidebarCollapsed]);

  const drawer = (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Toolbar sx={{ 
        minHeight: 72,
        display: 'flex',
        alignItems: 'center',
        justifyContent: sidebarCollapsed && !isMobile ? 'center' : 'center',
        px: sidebarCollapsed && !isMobile ? 1 : 2,
        background: (theme) => theme.palette.mode === 'light' 
          ? 'linear-gradient(135deg, #5C6BC0 0%, #3949AB 100%)'
          : 'linear-gradient(135deg, #1E293B 0%, #0F172A 100%)',
        color: '#fff'
      }}>
        {!(sidebarCollapsed && !isMobile) && (
          <Avatar sx={{ 
            bgcolor: 'rgba(255,255,255,0.2)', 
            width: 40, 
            height: 40,
            mr: 1.5
          }}>
            <BackupIcon />
          </Avatar>
        )}
        {!(sidebarCollapsed && !isMobile) && (
          <Typography 
            variant="h6" 
            noWrap 
            component="div"
            sx={{ fontWeight: 700, letterSpacing: 0.5 }}
          >
            AutoRestic
          </Typography>
        )}
        {sidebarCollapsed && !isMobile && (
          <Avatar sx={{ 
            bgcolor: 'rgba(255,255,255,0.2)', 
            width: 32, 
            height: 32,
          }}>
            <BackupIcon sx={{ fontSize: 20 }} />
          </Avatar>
        )}
      </Toolbar>
      <Divider />
      <List sx={{ flexGrow: 1, px: sidebarCollapsed && !isMobile ? 0.5 : 1, py: 1 }}>
        {menuItems.map((item) => {
          const isSelected = location.pathname === item.path;
          return (
            <ListItem key={item.text} disablePadding sx={{ mb: 0.5 }}>
              <ListItemButton
                component={Link}
                to={item.path}
                selected={isSelected}
                onClick={() => isMobile && setMobileOpen(false)}
                sx={{
                  borderRadius: 2,
                  mx: sidebarCollapsed && !isMobile ? 0.5 : 1,
                  py: 1.2,
                  justifyContent: sidebarCollapsed && !isMobile ? 'center' : 'flex-start',
                  '&.Mui-selected': {
                    backgroundColor: (theme) =>
                      theme.palette.mode === 'light'
                        ? 'rgba(92, 107, 192, 0.12)'
                        : 'rgba(129, 140, 248, 0.15)',
                    '&:hover': {
                      backgroundColor: (theme) =>
                        theme.palette.mode === 'light'
                          ? 'rgba(92, 107, 192, 0.18)'
                          : 'rgba(129, 140, 248, 0.2)',
                    },
                    '& .MuiListItemIcon-root': {
                      color: (theme) => theme.palette.primary.main,
                    },
                    '& .MuiListItemText-primary': {
                      fontWeight: 600,
                    },
                  },
                }}
              >
                <ListItemIcon sx={{ 
                  minWidth: sidebarCollapsed && !isMobile ? 0 : 44,
                  justifyContent: 'center'
                }}>
                  {item.icon}
                </ListItemIcon>
                {!(sidebarCollapsed && !isMobile) && (
                  <ListItemText 
                    primary={item.text} 
                    primaryTypographyProps={{
                      fontWeight: isSelected ? 600 : 400,
                    }}
                  />
                )}
              </ListItemButton>
            </ListItem>
          );
        })}
      </List>
      <Divider />
      <List sx={{ px: sidebarCollapsed && !isMobile ? 0.5 : 1, py: 1 }}>
        <ListItem disablePadding sx={{ mb: 0.5 }}>
          <ListItemButton
            onClick={handleLogout}
            sx={{
              borderRadius: 2,
              mx: sidebarCollapsed && !isMobile ? 0.5 : 1,
              py: 1.2,
              justifyContent: sidebarCollapsed && !isMobile ? 'center' : 'flex-start',
              '&:hover': {
                backgroundColor: (theme) =>
                  theme.palette.mode === 'light'
                    ? 'rgba(239, 68, 68, 0.1)'
                    : 'rgba(248, 113, 113, 0.15)',
              },
            }}
          >
            <ListItemIcon sx={{ 
              minWidth: sidebarCollapsed && !isMobile ? 0 : 44,
              justifyContent: 'center'
            }}>
              <LogoutIcon sx={{ color: (theme) => theme.palette.error.main }} />
            </ListItemIcon>
            {!(sidebarCollapsed && !isMobile) && (
              <ListItemText 
                primary="退出登录" 
                primaryTypographyProps={{
                  color: (theme) => theme.palette.error.main,
                }}
              />
            )}
          </ListItemButton>
        </ListItem>
      </List>
    </Box>
  );

  return (
    <Box sx={{ display: 'flex' }}>
      <AppBar
        position="fixed"
        sx={{
          width: { sm: `calc(100% - ${currentDrawerWidth}px)` },
          ml: { sm: `${currentDrawerWidth}px` },
          background: (theme) =>
            theme.palette.mode === 'light'
              ? '#fff'
              : theme.palette.background.paper,
          color: (theme) => theme.palette.text.primary,
          boxShadow: (theme) =>
            theme.palette.mode === 'light'
              ? '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)'
              : '0 1px 3px 0 rgba(0, 0, 0, 0.3), 0 1px 2px 0 rgba(0, 0, 0, 0.2)',
          transition: theme.transitions.create(['width', 'margin'], {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.leavingScreen,
          }),
        }}
      >
        <Toolbar sx={{ minHeight: 72 }}>
          <IconButton
            color="inherit"
            aria-label="toggle drawer"
            edge="start"
            onClick={handleDrawerToggle}
            sx={{ mr: 2 }}
          >
            {mobileOpen || (sidebarCollapsed && !isMobile) ? (
              isMobile ? <MenuIcon /> : <ChevronRightIcon />
            ) : (
              isMobile ? <MenuIcon /> : <ChevronLeftIcon />
            )}
          </IconButton>
          <Box sx={{ flexGrow: 1 }}>
            <Typography 
              variant="h5" 
              noWrap 
              component="div"
              sx={{ fontWeight: 600 }}
            >
              AutoRestic
            </Typography>
            <Typography 
              variant="body2" 
              color="text.secondary"
            >
              基于Restic的自动备份服务
            </Typography>
          </Box>
          <Tooltip title={mode === 'light' ? '切换到深色模式' : '切换到浅色模式'}>
            <IconButton onClick={toggleMode} sx={{ mx: 1 }}>
              {mode === 'light' ? <DarkModeIcon /> : <LightModeIcon />}
            </IconButton>
          </Tooltip>
          <Tooltip title="账户设置">
            <IconButton sx={{ ml: 1 }} onClick={() => setProfileDialogOpen(true)}>
              <SettingsIcon />
            </IconButton>
          </Tooltip>
        </Toolbar>
      </AppBar>
      <Box
        component="nav"
        sx={{ width: { sm: currentDrawerWidth }, flexShrink: { sm: 0 } }}
      >
        <Drawer
          variant="temporary"
          open={mobileOpen}
          onClose={() => setMobileOpen(false)}
          ModalProps={{
            keepMounted: true,
          }}
          sx={{
            display: { xs: 'block', sm: 'none' },
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
          }}
        >
          {drawer}
        </Drawer>
        <Drawer
          variant="permanent"
          sx={{
            display: { xs: 'none', sm: 'block' },
            '& .MuiDrawer-paper': { 
              boxSizing: 'border-box', 
              width: currentDrawerWidth,
              overflowX: 'hidden',
              transition: theme.transitions.create('width', {
                easing: theme.transitions.easing.sharp,
                duration: theme.transitions.duration.enteringScreen,
              }),
            },
          }}
          open
        >
          {drawer}
        </Drawer>
      </Box>
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          p: { xs: 2, sm: 3 },
          width: { sm: `calc(100% - ${currentDrawerWidth}px)` },
          minHeight: '100vh',
          transition: theme.transitions.create(['width', 'margin'], {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.leavingScreen,
          }),
        }}
      >
        <Toolbar sx={{ minHeight: 72 }} />
        <Outlet />
      </Box>

      <ProfileDialog
        open={profileDialogOpen}
        onClose={() => setProfileDialogOpen(false)}
        user={user}
        onSuccess={handleProfileSuccess}
      />

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar({ ...snackbar, open: false })}
      >
        <Alert severity={snackbar.severity}>{snackbar.message}</Alert>
      </Snackbar>
    </Box>
  );
};

export default Layout;
