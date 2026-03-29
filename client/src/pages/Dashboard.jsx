import React, { useEffect, useState } from 'react';
import {
  Box,
  Grid,
  Paper,
  Typography,
  Card,
  CardContent,
  Avatar,
  CircularProgress,
  alpha,
  Chip,
} from '@mui/material';
import {
  Storage as StorageIcon,
  Backup as BackupIcon,
  Schedule as ScheduleIcon,
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  TrendingUp as TrendingUpIcon,
  FolderOpen as FolderIcon,
  History as HistoryIcon,
  Timer as TimerIcon,
} from '@mui/icons-material';
import { logAPI, repositoryAPI, backupAPI, scheduleAPI } from '../api';

const StatCard = ({ title, value, icon, color }) => {
  return (
    <Card sx={{
      height: '100%',
      minHeight: 180,
      display: 'flex',
      flexDirection: 'column',
      width: '100%',
      flex: 1,
      transition: 'transform 0.2s, box-shadow 0.2s',
      '&:hover': {
        transform: 'translateY(-4px)',
        boxShadow: (theme) =>
          theme.palette.mode === 'light'
            ? '0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)'
            : '0 10px 25px -5px rgba(0, 0, 0, 0.3), 0 4px 6px -2px rgba(0, 0, 0, 0.2)',
      },
    }}>
      <CardContent sx={{ 
        p: 3,
        flexGrow: 1,
        display: 'flex',
        flexDirection: 'column',
        width: '100%'
      }}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexGrow: 1, width: '100%' }}>
          <Box sx={{ flexGrow: 1, minWidth: 0, maxWidth: 'calc(100% - 80px)' }}>
            <Typography
              variant="body2"
              color="text.secondary"
              gutterBottom
              sx={{ fontWeight: 500 }}
            >
              {title}
            </Typography>
            <Typography
              variant="h3"
              component="div"
              sx={{ 
                fontWeight: 700, 
                fontSize: { xs: '1.75rem', sm: '2rem', md: '2.5rem' },
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis'
              }}
            >
              {value}
            </Typography>
          </Box>
          <Box sx={{ ml: 2, flexShrink: 0 }}>
            <Avatar
              sx={{
                backgroundColor: (theme) => alpha(color, 0.12),
                color: color,
                width: { xs: 48, sm: 56 },
                height: { xs: 48, sm: 56 },
                boxShadow: (theme) =>
                  theme.palette.mode === 'light'
                    ? `0 4px 6px -1px ${alpha(color, 0.2)}, 0 2px 4px -1px ${alpha(color, 0.1)}`
                    : `0 4px 6px -1px ${alpha(color, 0.3)}, 0 2px 4px -1px ${alpha(color, 0.15)}`,
              }}
            >
              {icon}
            </Avatar>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
};

const InfoCard = ({ title, icon, color, children }) => {
  return (
    <Card sx={{ 
      height: '100%',
      minHeight: 350,
      display: 'flex',
      flexDirection: 'column',
      width: '100%'
    }}>
      <CardContent sx={{ 
        p: 3,
        flexGrow: 1,
        display: 'flex',
        flexDirection: 'column',
        width: '100%'
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
          <Avatar
            sx={{
              backgroundColor: (theme) => alpha(color, 0.12),
              color: color,
              width: 40,
              height: 40,
              mr: 2,
            }}
          >
            {icon}
          </Avatar>
          <Typography variant="h6" sx={{ fontWeight: 600, fontSize: { xs: '1.1rem', sm: '1.25rem' } }}>
            {title}
          </Typography>
        </Box>
        <Box sx={{ flexGrow: 1, width: '100%' }}>
          {children}
        </Box>
      </CardContent>
    </Card>
  );
};

const Dashboard = () => {
  const [stats, setStats] = useState(null);
  const [recentLogs, setRecentLogs] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [logStats, repos, backups, schedules, recentLogsRes] = await Promise.all([
          logAPI.getStats(),
          repositoryAPI.getAll(),
          backupAPI.getAll(),
          scheduleAPI.getAll(),
          logAPI.getAll({ limit: 5 }),
        ]);

        setStats({
          logs: logStats.data,
          repositories: repos.data.length,
          backups: backups.data.length,
          schedules: schedules.data.length,
        });
        setRecentLogs(recentLogsRes.data || []);
      } catch (error) {
        console.error('Failed to fetch dashboard data:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  if (loading) {
    return (
      <Box sx={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '50vh',
      }}>
        <CircularProgress size={48} />
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ mb: 4 }}>
        <Typography 
          variant="h3" 
          gutterBottom 
          sx={{ fontWeight: 700, fontSize: { xs: '1.75rem', sm: '2.125rem' } }}
        >
          仪表盘
        </Typography>
        <Typography 
          variant="body1" 
          color="text.secondary"
        >
          欢迎回来！这是您的备份系统概览
        </Typography>
      </Box>

      <Box sx={{ mb: 4, display: 'flex', flexWrap: 'wrap', gap: 3 }}>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="仓库总数"
            value={stats?.repositories || 0}
            icon={<StorageIcon sx={{ fontSize: 28 }} />}
            color="#5C6BC0"
          />
        </Box>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="备份任务"
            value={stats?.backups || 0}
            icon={<BackupIcon sx={{ fontSize: 28 }} />}
            color="#10B981"
          />
        </Box>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="定时任务"
            value={stats?.schedules || 0}
            icon={<ScheduleIcon sx={{ fontSize: 28 }} />}
            color="#F59E0B"
          />
        </Box>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="成功备份"
            value={stats?.logs?.success || 0}
            icon={<SuccessIcon sx={{ fontSize: 28 }} />}
            color="#10B981"
          />
        </Box>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="失败备份"
            value={stats?.logs?.failed || 0}
            icon={<ErrorIcon sx={{ fontSize: 28 }} />}
            color="#EF4444"
          />
        </Box>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(33.333333% - 16px)' }, flexGrow: 0, flexShrink: 0, display: 'flex' }}>
          <StatCard
            title="总备份次数"
            value={stats?.logs?.total || 0}
            icon={<HistoryIcon sx={{ fontSize: 28 }} />}
            color="#3B82F6"
          />
        </Box>
      </Box>

      <Grid container spacing={3}>
        <Grid item xs={12} md={6} sx={{ display: 'flex', width: '100%' }}>
          <InfoCard
            title="最近操作"
            icon={<HistoryIcon />}
            color="#5C6BC0"
          >
            {recentLogs.length === 0 ? (
              <Box sx={{ flexGrow: 1, width: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Typography color="text.secondary" sx={{ py: 4 }}>
                  暂无操作记录
                </Typography>
              </Box>
            ) : (
              <Box sx={{ width: '100%' }}>
                {recentLogs.map((log, index) => (
                  <Box
                    key={log.id}
                    sx={{
                      display: 'flex',
                      alignItems: 'flex-start',
                      py: 1.5,
                      borderBottom: (theme) =>
                        index < recentLogs.length - 1
                          ? `1px solid ${theme.palette.divider}`
                          : 'none',
                    }}
                  >
                    <Chip
                      label={log.status === 'success' ? '成功' : '失败'}
                      color={log.status === 'success' ? 'success' : 'error'}
                      size="small"
                      sx={{ mr: 2, minWidth: 60, flexShrink: 0, mt: 0.5 }}
                    />
                    <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                      <Typography
                        variant="body2"
                        sx={{ fontWeight: 500, wordBreak: 'break-word' }}
                      >
                        {log.message || '备份任务'}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {new Date(log.created_at).toLocaleString()}
                      </Typography>
                    </Box>
                  </Box>
                ))}
              </Box>
            )}
          </InfoCard>
        </Grid>

        <Grid item xs={12} md={6} sx={{ display: 'flex', width: '100%' }}>
          <InfoCard
            title="快速操作"
            icon={<TimerIcon />}
            color="#F59E0B"
          >
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <Paper
                  sx={{
                    p: 2,
                    display: 'flex',
                    alignItems: 'center',
                    cursor: 'pointer',
                    transition: 'background-color 0.2s',
                    '&:hover': {
                      backgroundColor: (theme) => alpha('#5C6BC0', 0.08),
                    },
                  }}
                >
                  <StorageIcon sx={{ color: '#5C6BC0', mr: 2 }} />
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      管理仓库
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      配置备份存储位置
                    </Typography>
                  </Box>
                </Paper>
              </Grid>
              <Grid item xs={12}>
                <Paper
                  sx={{
                    p: 2,
                    display: 'flex',
                    alignItems: 'center',
                    cursor: 'pointer',
                    transition: 'background-color 0.2s',
                    '&:hover': {
                      backgroundColor: (theme) => alpha('#10B981', 0.08),
                    },
                  }}
                >
                  <BackupIcon sx={{ color: '#10B981', mr: 2 }} />
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      创建备份
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      立即执行备份任务
                    </Typography>
                  </Box>
                </Paper>
              </Grid>
              <Grid item xs={12}>
                <Paper
                  sx={{
                    p: 2,
                    display: 'flex',
                    alignItems: 'center',
                    cursor: 'pointer',
                    transition: 'background-color 0.2s',
                    '&:hover': {
                      backgroundColor: (theme) => alpha('#F59E0B', 0.08),
                    },
                  }}
                >
                  <ScheduleIcon sx={{ color: '#F59E0B', mr: 2 }} />
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      设置定时
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      配置自动备份计划
                    </Typography>
                  </Box>
                </Paper>
              </Grid>
              <Grid item xs={12}>
                <Paper
                  sx={{
                    p: 2,
                    display: 'flex',
                    alignItems: 'center',
                    cursor: 'pointer',
                    transition: 'background-color 0.2s',
                    '&:hover': {
                      backgroundColor: (theme) => alpha('#3B82F6', 0.08),
                    },
                  }}
                >
                  <FolderIcon sx={{ color: '#3B82F6', mr: 2 }} />
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600 }}>
                      查看快照
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      浏览和恢复备份
                    </Typography>
                  </Box>
                </Paper>
              </Grid>
            </Grid>
          </InfoCard>
        </Grid>
      </Grid>
    </Box>
  );
};

export default Dashboard;
