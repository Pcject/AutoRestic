import React, { useEffect, useState } from 'react';
import {
  Box,
  Typography,
  Chip,
  Card,
  CardContent,
  Grid,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Dialog,
  DialogTitle,
  DialogContent,
  Tabs,
  Tab,
  IconButton,
  FormControlLabel,
  Checkbox,
} from '@mui/material';
import {
  CheckCircle as SuccessIcon,
  Error as ErrorIcon,
  Schedule as RunningIcon,
  Close as CloseIcon,
} from '@mui/icons-material';
import { logAPI } from '../api';

const Logs = () => {
  const [logs, setLogs] = useState([]);
  const [stats, setStats] = useState(null);
  const [filter, setFilter] = useState({ type: '', status: '', backup_id: '', exclude_cached: false });
  const [loading, setLoading] = useState(true);
  const [selectedLog, setSelectedLog] = useState(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [tabValue, setTabValue] = useState(0);

  const renderTerminalOutput = (content) => {
    if (!content) return '无输出';

    return (
      <Box
        sx={{
          fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
          fontSize: '0.875rem',
          lineHeight: 1.5,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          color: '#e0e0e0',
        }}
      >
        {content}
      </Box>
    );
  };

  const getOperationTypeLabel = (type) => {
    const labels = {
      'backup': 'backup',
      'snapshots': 'snapshots',
      'ls': 'ls',
      'restore': 'restore',
      'check': 'check',
      'prune': 'prune',
      'stats': 'stats',
      'forget': 'forget',
      'unlock': 'unlock',
      'init': 'init'
    };
    return labels[type] || type;
  };

  useEffect(() => {
    fetchData();
  }, [filter]);

  const fetchData = async () => {
    try {
      const [logsRes, statsRes] = await Promise.all([
        logAPI.getAll(filter),
        logAPI.getStats(),
      ]);
      setLogs(logsRes.data);
      setStats(statsRes.data);
    } catch (error) {
      console.error('Failed to fetch logs:', error);
    } finally {
      setLoading(false);
    }
  };

  const getStatusIcon = (status) => {
    switch (status) {
      case 'success':
        return <SuccessIcon color="success" />;
      case 'failed':
        return <ErrorIcon color="error" />;
      case 'running':
        return <RunningIcon color="primary" />;
      default:
        return null;
    }
  };

  const getStatusColor = (status) => {
    switch (status) {
      case 'success':
        return 'success';
      case 'failed':
        return 'error';
      case 'running':
        return 'primary';
      default:
        return 'default';
    }
  };

  const formatDuration = (ms) => {
    if (!ms) return '-';
    const seconds = Math.floor(ms / 1000);
    if (seconds < 60) return `${seconds}s`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
    const hours = Math.floor(minutes / 60);
    return `${hours}h ${minutes % 60}m`;
  };

  const handleRowClick = (log) => {
    setSelectedLog(log);
    setTabValue(0);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
    setSelectedLog(null);
  };

  const handleTabChange = (event, newValue) => {
    setTabValue(newValue);
  };

  return (
    <Box>
      <Typography variant="h4" gutterBottom>
        日志记录
      </Typography>

      <Grid container spacing={3} sx={{ mb: 3 }}>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                总备份次数
              </Typography>
              <Typography variant="h4" component="div">
                {stats?.total || 0}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                成功次数
              </Typography>
              <Typography variant="h4" component="div" sx={{ color: '#2e7d32' }}>
                {stats?.success || 0}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                失败次数
              </Typography>
              <Typography variant="h4" component="div" sx={{ color: '#d32f2f' }}>
                {stats?.failed || 0}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                平均耗时
              </Typography>
              <Typography variant="h4" component="div">
                {formatDuration(stats?.avg_duration)}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Paper sx={{ mb: 2, p: 2 }}>
        <Grid container spacing={2} alignItems="center">
          <Grid item xs={12} sm={6} md={4}>
            <FormControl fullWidth size="small">
              <InputLabel>操作类型</InputLabel>
              <Select
                value={filter.type}
                label="操作类型"
                onChange={(e) => setFilter({ ...filter, type: e.target.value })}
                MenuProps={{
                  PaperProps: {
                    sx: {
                      maxHeight: 300,
                    },
                  },
                }}
              >
                <MenuItem value="">全部</MenuItem>
                <MenuItem value="backup">backup</MenuItem>
                <MenuItem value="snapshots">snapshots</MenuItem>
                <MenuItem value="ls">ls</MenuItem>
                <MenuItem value="restore">restore</MenuItem>
                <MenuItem value="check">check</MenuItem>
                <MenuItem value="prune">prune</MenuItem>
                <MenuItem value="stats">stats</MenuItem>
                <MenuItem value="forget">forget</MenuItem>
                <MenuItem value="unlock">unlock</MenuItem>
                <MenuItem value="init">init</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={6} md={4}>
            <FormControl fullWidth size="small">
              <InputLabel>状态筛选</InputLabel>
              <Select
                value={filter.status}
                label="状态筛选"
                onChange={(e) => setFilter({ ...filter, status: e.target.value })}
              >
                <MenuItem value="">全部</MenuItem>
                <MenuItem value="success">成功</MenuItem>
                <MenuItem value="failed">失败</MenuItem>
                <MenuItem value="running">运行中</MenuItem>
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} sm={12} md={4}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={filter.exclude_cached}
                  onChange={(e) => setFilter({ ...filter, exclude_cached: e.target.checked })}
                />
              }
              label="过滤缓存消息"
            />
          </Grid>
        </Grid>
      </Paper>

      <TableContainer component={Paper}>
        <Table stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>操作类型</TableCell>
              <TableCell>仓库ID</TableCell>
              <TableCell>备份ID</TableCell>
              <TableCell>状态</TableCell>
              <TableCell>消息</TableCell>
              <TableCell>快照ID</TableCell>
              <TableCell>耗时</TableCell>
              <TableCell>开始时间</TableCell>
              <TableCell>完成时间</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={10} align="center">
                  加载中...
                </TableCell>
              </TableRow>
            ) : logs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={10} align="center">
                  暂无日志记录
                </TableCell>
              </TableRow>
            ) : (
              logs.map((log) => (
                <TableRow 
                  key={log.id} 
                  hover 
                  onClick={() => handleRowClick(log)} 
                  sx={{ cursor: 'pointer' }}
                >
                  <TableCell>{log.id}</TableCell>
                  <TableCell>
                    <Chip
                      label={getOperationTypeLabel(log.type)}
                      size="small"
                      variant="outlined"
                    />
                  </TableCell>
                  <TableCell>{log.repository_id || '-'}</TableCell>
                  <TableCell>{log.backup_id || '-'}</TableCell>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      {getStatusIcon(log.status)}
                      <Chip
                        label={log.status}
                        color={getStatusColor(log.status)}
                        size="small"
                      />
                    </Box>
                  </TableCell>
                  <TableCell>{log.message}</TableCell>
                  <TableCell>
                    <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                      {log.snapshot_id || '-'}
                    </Typography>
                  </TableCell>
                  <TableCell>{formatDuration(log.duration)}</TableCell>
                  <TableCell>{new Date(log.started_at).toLocaleString('zh-CN')}</TableCell>
                  <TableCell>
                    {log.completed_at ? new Date(log.completed_at).toLocaleString('zh-CN') : '-'}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={dialogOpen} onClose={handleDialogClose} maxWidth="lg" fullWidth>
        <DialogTitle>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="h6">日志详情 #{selectedLog?.id}</Typography>
            <IconButton onClick={handleDialogClose} size="small">
              <CloseIcon />
            </IconButton>
          </Box>
        </DialogTitle>
        <DialogContent>
          {selectedLog && (
            <>
              <Box sx={{ mb: 2 }}>
                <Grid container spacing={2}>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">操作类型</Typography>
                    <Chip
                      label={getOperationTypeLabel(selectedLog.type)}
                      size="small"
                      variant="outlined"
                    />
                  </Grid>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">仓库ID</Typography>
                    <Typography>{selectedLog.repository_id || '-'}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">备份ID</Typography>
                    <Typography>{selectedLog.backup_id || '-'}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">状态</Typography>
                    <Chip
                      label={selectedLog.status}
                      color={getStatusColor(selectedLog.status)}
                      size="small"
                    />
                  </Grid>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">快照ID</Typography>
                    <Typography sx={{ fontFamily: 'monospace' }}>{selectedLog.snapshot_id || '-'}</Typography>
                  </Grid>
                  <Grid item xs={12} sm={6} md={3}>
                    <Typography variant="subtitle2" color="textSecondary">耗时</Typography>
                    <Typography>{formatDuration(selectedLog.duration)}</Typography>
                  </Grid>
                </Grid>
              </Box>

              {selectedLog.restic_command && (
                <>
                  <Tabs value={tabValue} onChange={handleTabChange}>
                    <Tab label="命令" />
                    <Tab label="标准输出 (stdout)" />
                    <Tab label="错误输出 (stderr)" />
                  </Tabs>

                  <Box sx={{ mt: 2 }}>
                    {tabValue === 0 && (
                      <Box
                        sx={{
                          bgcolor: '#1e1e1e',
                          borderRadius: 1,
                          p: 2,
                          overflow: 'auto',
                          maxHeight: '60vh',
                        }}
                      >
                        <Box
                          sx={{
                            fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
                            fontSize: '0.875rem',
                            lineHeight: 1.5,
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                            color: '#4ec9b0',
                          }}
                        >
                          $ {selectedLog.restic_command}
                        </Box>
                      </Box>
                    )}
                    {tabValue === 1 && (
                      <Box
                        sx={{
                          bgcolor: '#1e1e1e',
                          borderRadius: 1,
                          p: 2,
                          overflow: 'auto',
                          maxHeight: '60vh',
                        }}
                      >
                        {renderTerminalOutput(selectedLog.restic_stdout_formatted || selectedLog.restic_stdout)}
                      </Box>
                    )}
                    {tabValue === 2 && (
                      <Box
                        sx={{
                          bgcolor: '#1e1e1e',
                          borderRadius: 1,
                          p: 2,
                          overflow: 'auto',
                          maxHeight: '60vh',
                        }}
                      >
                        <Box
                          sx={{
                            fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
                            fontSize: '0.875rem',
                            lineHeight: 1.5,
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-all',
                            color: '#f14c4c',
                          }}
                        >
                          {selectedLog.restic_stderr_formatted || selectedLog.restic_stderr || '无输出'}
                        </Box>
                      </Box>
                    )}
                  </Box>
                </>
              )}

              {!selectedLog.restic_command && (
                <Typography variant="body2" color="textSecondary">
                  此日志无 restic 命令详情信息
                </Typography>
              )}
            </>
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
};

export default Logs;