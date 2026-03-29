import React, { useEffect, useState } from 'react';
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  CircularProgress,
  TablePagination,
  IconButton,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Divider,
  useTheme,
  useMediaQuery,
  Grid,
} from '@mui/material';
import {
  Refresh as RefreshIcon,
  Info as InfoIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import { taskAPI } from '../api';

const TaskLogsDialog = ({ open, onClose, task }) => {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open && task) {
      fetchLogs();
    }
  }, [open, task]);

  const fetchLogs = async () => {
    setLoading(true);
    try {
      const response = await taskAPI.getLogs(task.id);
      setLogs(response.data);
    } catch (error) {
      console.error('Failed to fetch task logs:', error);
    } finally {
      setLoading(false);
    }
  };

  const getLogColor = (level) => {
    switch (level) {
      case 'error':
        return 'error';
      case 'warning':
        return 'warning';
      case 'info':
        return 'info';
      default:
        return 'default';
    }
  };

  const hasOutput = task?.output || task?.error_message;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle sx={{ 
        pb: 1,
        background: (theme) => theme.palette.mode === 'light' 
          ? 'linear-gradient(135deg, #5C6BC0 0%, #3949AB 100%)'
          : 'linear-gradient(135deg, #1E293B 0%, #0F172A 100%)',
        color: '#fff',
        mx: -3,
        mt: -2,
        px: 3,
        py: 2
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          任务日志 - #{task?.id}
          <IconButton onClick={fetchLogs} sx={{ color: '#fff' }}>
            <RefreshIcon />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent sx={{ pt: 3 }}>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress size={48} />
          </Box>
        ) : (
          <Box>
            {hasOutput && (
              <Box sx={{ mb: 3 }}>
                <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
                  Restic 命令输出
                </Typography>
                <Grid container spacing={2}>
                  {task?.output && (
                    <Grid item xs={12} md={6}>
                      <Paper sx={{ p: 2, backgroundColor: (theme) => theme.palette.mode === 'light' ? '#f0fdf4' : '#14532d' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600, color: (theme) => theme.palette.mode === 'light' ? '#166534' : '#86efac' }}>
                          STDOUT (标准输出)
                        </Typography>
                        <pre style={{ 
                          fontSize: '0.75rem', 
                          overflow: 'auto', 
                          margin: 0,
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                          fontFamily: 'monospace',
                          maxHeight: '200px'
                        }}>
                          {task.output}
                        </pre>
                      </Paper>
                    </Grid>
                  )}
                  {task?.error_message && (
                    <Grid item xs={12} md={6}>
                      <Paper sx={{ p: 2, backgroundColor: (theme) => theme.palette.mode === 'light' ? '#fef2f2' : '#7f1d1d' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600, color: (theme) => theme.palette.mode === 'light' ? '#991b1b' : '#fca5a5' }}>
                          STDERR (错误输出)
                        </Typography>
                        <pre style={{ 
                          fontSize: '0.75rem', 
                          overflow: 'auto', 
                          margin: 0,
                          whiteSpace: 'pre-wrap',
                          wordBreak: 'break-all',
                          fontFamily: 'monospace',
                          maxHeight: '200px'
                        }}>
                          {task.error_message}
                        </pre>
                      </Paper>
                    </Grid>
                  )}
                </Grid>
                <Divider sx={{ my: 3 }} />
              </Box>
            )}

            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              任务日志
            </Typography>
            {logs.length === 0 ? (
              <Typography color="text.secondary" align="center" sx={{ py: 4 }}>
                暂无日志
              </Typography>
            ) : (
              <Box>
                {logs.map((log) => (
                  <Box key={log.id} sx={{ mb: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'flex-start' }}>
                      <Chip
                        label={log.level.toUpperCase()}
                        size="small"
                        color={getLogColor(log.level)}
                        sx={{ mr: 2, mt: 0.5 }}
                      />
                      <Box sx={{ flexGrow: 1 }}>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                          {log.message}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {new Date(log.created_at).toLocaleString()}
                        </Typography>
                      </Box>
                    </Box>
                    <Divider sx={{ mt: 1 }} />
                  </Box>
                ))}
              </Box>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 3 }}>
        <Button onClick={onClose} variant="outlined">关闭</Button>
      </DialogActions>
    </Dialog>
  );
};

const Tasks = () => {
  const [tasks, setTasks] = useState([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [selectedTask, setSelectedTask] = useState(null);
  const [logsDialogOpen, setLogsDialogOpen] = useState(false);
  const [typeFilter, setTypeFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  const fetchTasks = async () => {
    setLoading(true);
    try {
      const params = {};
      if (typeFilter) params.type = typeFilter;
      if (statusFilter) params.status = statusFilter;
      
      const response = await taskAPI.getAll(params);
      setTasks(response.data);
    } catch (error) {
      console.error('Failed to fetch tasks:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
  }, [typeFilter, statusFilter]);

  const getStatusColor = (status) => {
    switch (status) {
      case 'pending':
        return 'default';
      case 'running':
        return 'primary';
      case 'completed':
        return 'success';
      case 'failed':
        return 'error';
      default:
        return 'default';
    }
  };

  const getStatusText = (status) => {
    switch (status) {
      case 'pending':
        return '待处理';
      case 'running':
        return '运行中';
      case 'completed':
        return '已完成';
      case 'failed':
        return '失败';
      default:
        return status;
    }
  };

  const getTypeText = (type) => {
    switch (type) {
      case 'backup':
        return '备份';
      case 'restore':
        return '恢复';
      case 'check':
        return '检查';
      case 'prune':
        return '清理';
      case 'init':
        return '初始化';
      default:
        return type;
    }
  };

  const handleViewLogs = (task) => {
    setSelectedTask(task);
    setLogsDialogOpen(true);
  };

  if (loading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ 
        display: 'flex', 
        flexDirection: { xs: 'column', sm: 'row' },
        justifyContent: 'space-between', 
        alignItems: { xs: 'stretch', sm: 'center' }, 
        mb: 3,
        gap: { xs: 2, sm: 0 }
      }}>
        <Typography variant="h4" sx={{ fontWeight: 600, fontSize: { xs: '1.5rem', sm: '2.125rem' } }}>
          任务管理
        </Typography>
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={fetchTasks}
          fullWidth={isMobile}
        >
          刷新
        </Button>
      </Box>

      {isMobile ? (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {tasks
            .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
            .map((task) => (
              <Paper key={task.id} sx={{ p: 2 }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                  <Box>
                    <Typography variant="h6" sx={{ fontWeight: 600 }}>
                      #{task.id} - {getTypeText(task.type)}
                    </Typography>
                  </Box>
                  <Chip
                    label={getStatusText(task.status)}
                    color={getStatusColor(task.status)}
                    size="small"
                  />
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  开始: {new Date(task.started_at).toLocaleString()}
                </Typography>
                {task.completed_at && (
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    结束: {new Date(task.completed_at).toLocaleString()}
                  </Typography>
                )}
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={<InfoIcon />}
                  onClick={() => handleViewLogs(task)}
                  fullWidth
                >
                  查看日志
                </Button>
              </Paper>
            ))}
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={tasks.length}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
          />
        </Box>
      ) : (
        <TableContainer component={Paper}>
          <Table stickyHeader>
            <TableHead>
              <TableRow>
                <TableCell>ID</TableCell>
                <TableCell>类型</TableCell>
                <TableCell>状态</TableCell>
                <TableCell>开始时间</TableCell>
                <TableCell>结束时间</TableCell>
                <TableCell 
                  align="right"
                  sx={{ 
                    position: 'sticky', 
                    right: 0, 
                    backgroundColor: (theme) => theme.palette.background.paper,
                    zIndex: 1 
                  }}
                >
                  操作
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {tasks
                .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
                .map((task) => (
                  <TableRow key={task.id}>
                    <TableCell>#{task.id}</TableCell>
                    <TableCell>
                      <Chip label={getTypeText(task.type)} size="small" />
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={getStatusText(task.status)}
                        color={getStatusColor(task.status)}
                        size="small"
                      />
                    </TableCell>
                    <TableCell>
                      {new Date(task.started_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {task.completed_at ? new Date(task.completed_at).toLocaleString() : '-'}
                    </TableCell>
                    <TableCell 
                      align="right"
                      sx={{ 
                        position: 'sticky', 
                        right: 0, 
                        backgroundColor: (theme) => theme.palette.background.paper,
                        zIndex: 1 
                      }}
                    >
                      <Box sx={{ display: 'flex', gap: 1 }}>
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={<InfoIcon />}
                          onClick={() => handleViewLogs(task)}
                        >
                          查看日志
                        </Button>
                      </Box>
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={tasks.length}
            rowsPerPage={rowsPerPage}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            onRowsPerPageChange={(e) => {
              setRowsPerPage(parseInt(e.target.value, 10));
              setPage(0);
            }}
          />
        </TableContainer>
      )}

      <TaskLogsDialog
        open={logsDialogOpen}
        onClose={() => setLogsDialogOpen(false)}
        task={selectedTask}
      />
    </Box>
  );
};

export default Tasks;
