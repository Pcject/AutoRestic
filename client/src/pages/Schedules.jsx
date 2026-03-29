import React, { useEffect, useState } from 'react';
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  IconButton,
  Typography,
  Chip,
  Alert,
  Snackbar,
  Switch,
  FormControlLabel,
  FormControl,
  FormGroup,
  Checkbox,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  ToggleOn as EnableIcon,
  ToggleOff as DisableIcon,
} from '@mui/icons-material';
import { DataGrid } from '@mui/x-data-grid';
import { scheduleAPI, backupAPI } from '../api';

const ScheduleDialog = ({ open, onClose, schedule, backups, onSubmit }) => {
  const [formData, setFormData] = useState({
    backup_ids: [],
    cron_expression: '0 2 * * *',
    enabled: true,
  });

  const cronExamples = [
    { label: '每天凌晨2点', value: '0 2 * * *' },
    { label: '每天中午12点', value: '0 12 * * *' },
    { label: '每周日凌晨3点', value: '0 3 * * 0' },
    { label: '每月1号凌晨2点', value: '0 2 1 * *' },
    { label: '每6小时', value: '0 */6 * * *' },
  ];

  const groupedBackups = React.useMemo(() => {
    const groups = {};
    backups.forEach(backup => {
      const repoName = backup.repository_name || '未分组';
      if (!groups[repoName]) {
        groups[repoName] = [];
      }
      groups[repoName].push(backup);
    });
    return groups;
  }, [backups]);

  const handleSelectAll = (event) => {
    if (event.target.checked) {
      setFormData(prev => ({ 
        ...prev, 
        backup_ids: backups.map(b => Number(b.id)) 
      }));
    } else {
      setFormData(prev => ({ 
        ...prev, 
        backup_ids: [] 
      }));
    }
  };

  const handleSelectRepository = (repoName, isChecked) => {
    const repoBackups = groupedBackups[repoName] || [];
    setFormData(prev => {
      if (isChecked) {
        const newIds = [...prev.backup_ids];
        repoBackups.forEach(backup => {
          const numId = Number(backup.id);
          if (!newIds.includes(numId)) {
            newIds.push(numId);
          }
        });
        return { ...prev, backup_ids: newIds };
      } else {
        const repoIds = repoBackups.map(b => Number(b.id));
        return { 
          ...prev, 
          backup_ids: prev.backup_ids.filter(id => !repoIds.includes(id))
        };
      }
    });
  };

  const handleBackupToggle = (backupId, isChecked) => {
    setFormData(prev => {
      if (isChecked) {
        return {
          ...prev,
          backup_ids: [...prev.backup_ids, Number(backupId)]
        };
      } else {
        return {
          ...prev,
          backup_ids: prev.backup_ids.filter(id => id !== Number(backupId))
        };
      }
    });
  };

  useEffect(() => {
    if (open) {
      if (schedule) {
        let backupIds = [];
        try {
          if (schedule.backup_ids) {
            if (Array.isArray(schedule.backup_ids)) {
              backupIds = schedule.backup_ids.map(id => Number(id));
            } else if (typeof schedule.backup_ids === 'string') {
              try {
                const parsed = JSON.parse(schedule.backup_ids);
                if (Array.isArray(parsed)) {
                  backupIds = parsed.map(id => Number(id));
                } else if (typeof parsed === 'number') {
                  backupIds = [parsed];
                } else if (typeof parsed === 'string' && !isNaN(parseInt(parsed))) {
                  backupIds = [parseInt(parsed)];
                }
              } catch (parseError) {
                if (schedule.backup_id) {
                  backupIds = [Number(schedule.backup_id)];
                }
              }
            } else if (typeof schedule.backup_ids === 'number') {
              backupIds = [schedule.backup_ids];
            }
          } else if (schedule.backup_id) {
            backupIds = [Number(schedule.backup_id)];
          }
        } catch (error) {
          console.error('Failed to parse backup_ids:', error);
        }
        setFormData({
          backup_ids: backupIds,
          cron_expression: schedule.cron_expression,
          enabled: schedule.enabled === 1,
        });
      } else {
        setFormData({
          backup_ids: [],
          cron_expression: '0 2 * * *',
          enabled: true,
        });
      }
    }
  }, [open, schedule]);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (formData.backup_ids.length === 0) {
      alert('请至少选择一个备份任务');
      return;
    }
    onSubmit(formData);
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{schedule ? '编辑定时任务' : '新建定时任务'}</DialogTitle>
      <form onSubmit={handleSubmit}>
        <DialogContent>
          <Typography variant="subtitle1" gutterBottom sx={{ mt: 1, mb: 1 }}>
            选择备份任务
          </Typography>
          {backups.length > 0 && (
            <FormControlLabel
              control={
                <Checkbox
                  checked={formData.backup_ids.length === backups.length && backups.length > 0}
                  indeterminate={formData.backup_ids.length > 0 && formData.backup_ids.length < backups.length}
                  onChange={handleSelectAll}
                />
              }
              label="全选"
              sx={{ mb: 1 }}
            />
          )}
          <FormControl component="fieldset" sx={{ mb: 2 }}>
            <FormGroup>
              {Object.keys(groupedBackups).map((repoName) => {
                const repoBackups = groupedBackups[repoName];
                const allSelected = repoBackups.every(b => formData.backup_ids.includes(Number(b.id)));
                const someSelected = repoBackups.some(b => formData.backup_ids.includes(Number(b.id)));
                return (
                  <Box key={repoName} sx={{ mb: 1 }}>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={allSelected}
                          indeterminate={someSelected && !allSelected}
                          onChange={(e) => handleSelectRepository(repoName, e.target.checked)}
                        />
                      }
                      label={
                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                          <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                            {repoName}
                          </Typography>
                          <Chip 
                            label={`${repoBackups.length}`} 
                            size="small" 
                            sx={{ ml: 1, fontSize: '0.7rem' }}
                          />
                        </Box>
                      }
                      sx={{ 
                        mb: 0.5, 
                        '& .MuiCheckbox-root': { color: '#5C6BC0' }
                      }}
                    />
                    <Box sx={{ pl: 4 }}>
                      {repoBackups.map((backup) => (
                        <FormControlLabel
                          key={backup.id}
                          control={
                            <Checkbox
                              checked={formData.backup_ids.includes(Number(backup.id))}
                              onChange={(e) => handleBackupToggle(backup.id, e.target.checked)}
                              size="small"
                            />
                          }
                          label={backup.name}
                          sx={{ mb: 0.25 }}
                        />
                      ))}
                    </Box>
                  </Box>
                );
              })}
            </FormGroup>
          </FormControl>

          <TextField
            fullWidth
            label="Cron表达式"
            value={formData.cron_expression}
            onChange={(e) => setFormData({ ...formData, cron_expression: e.target.value })}
            required
            helperText="格式: 分 时 日 月 周 (例如: 0 2 * * * 表示每天凌晨2点)"
            sx={{ mb: 2 }}
          />

          <Typography variant="subtitle2" gutterBottom>
            常用Cron表达式:
          </Typography>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mb: 2 }}>
            {cronExamples.map((example) => (
              <Chip
                key={example.value}
                label={example.label}
                onClick={() => setFormData({ ...formData, cron_expression: example.value })}
                clickable
                variant="outlined"
              />
            ))}
          </Box>

          <FormControlLabel
            control={
              <Switch
                checked={formData.enabled}
                onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
              />
            }
            label="启用定时任务"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose}>取消</Button>
          <Button type="submit" variant="contained">保存</Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};

const Schedules = () => {
  const [schedules, setSchedules] = useState([]);
  const [backups, setBackups] = useState([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedSchedule, setSelectedSchedule] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });
  const [loading, setLoading] = useState(true);

  const columns = [
    { field: 'id', headerName: 'ID', width: 45 },
    { field: 'backup_name', headerName: '备份任务', width: 100 },
    { field: 'repository_name', headerName: '仓库', width: 90 },
    { field: 'cron_expression', headerName: 'Cron表达式', width: 110 },
    {
      field: 'enabled',
      headerName: '状态',
      width: 65,
      renderCell: (params) => (
        <Chip
          label={params.value ? '启用' : '禁用'}
          color={params.value ? 'success' : 'default'}
          size="small"
        />
      ),
    },
    { field: 'last_run', headerName: '上次运行', width: 110 },
    { field: 'next_run', headerName: '下次运行', width: 110 },
    {
      field: 'actions',
      headerName: '操作',
      width: 125,
      sortable: false,
      renderCell: (params) => (
        <Box sx={{ display: 'flex', gap: 0.25 }}>
          <IconButton
            onClick={() => handleToggle(params.row.id)}
            title={params.row.enabled ? '禁用' : '启用'}
            size="small"
          >
            {params.row.enabled ? <EnableIcon /> : <DisableIcon />}
          </IconButton>
          <IconButton onClick={() => handleEdit(params.row)} title="编辑" size="small">
            <EditIcon />
          </IconButton>
          <IconButton onClick={() => handleDelete(params.row.id)} title="删除" size="small">
            <DeleteIcon />
          </IconButton>
        </Box>
      ),
    },
  ];

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [schedulesRes, backupsRes] = await Promise.all([
        scheduleAPI.getAll(),
        backupAPI.getAll(),
      ]);
      setSchedules(schedulesRes.data);
      setBackups(backupsRes.data);
    } catch (error) {
      showSnackbar('获取数据失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleToggle = async (id) => {
    try {
      await scheduleAPI.toggle(id);
      showSnackbar('定时任务状态已更新', 'success');
      fetchData();
    } catch (error) {
      showSnackbar('更新状态失败', 'error');
    }
  };

  const handleEdit = (schedule) => {
    setSelectedSchedule(schedule);
    setDialogOpen(true);
  };

  const handleDelete = async (id) => {
    if (window.confirm('确定要删除这个定时任务吗？')) {
      try {
        await scheduleAPI.delete(id);
        showSnackbar('定时任务删除成功', 'success');
        fetchData();
      } catch (error) {
        showSnackbar('定时任务删除失败', 'error');
      }
    }
  };

  const handleSubmit = async (data) => {
    try {
      if (selectedSchedule) {
        await scheduleAPI.update(selectedSchedule.id, data);
        showSnackbar('定时任务更新成功', 'success');
      } else {
        await scheduleAPI.create(data);
        showSnackbar('定时任务创建成功', 'success');
      }
      setDialogOpen(false);
      setSelectedSchedule(null);
      fetchData();
    } catch (error) {
      showSnackbar('操作失败: ' + (error.response?.data?.error || error.message), 'error');
    }
  };

  const showSnackbar = (message, severity) => {
    setSnackbar({ open: true, message, severity });
  };

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
        <Typography variant="h4" sx={{ fontWeight: 600, fontSize: { xs: '1.5rem', sm: '2.125rem' } }}>定时任务</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => {
            setSelectedSchedule(null);
            setDialogOpen(true);
          }}
        >
          新建定时任务
        </Button>
      </Box>

      <Box sx={{ height: 500, width: '100%' }}>
        <DataGrid
          rows={schedules}
          columns={columns}
          initialState={{
            pagination: {
              paginationModel: { pageSize: 10 },
            },
            columnPinning: {
              right: ['actions'],
            },
          }}
          pageSizeOptions={[5, 10, 25]}
          loading={loading}
          disableRowSelectionOnClick
          getRowId={(row) => row.id}
        />
      </Box>

      <ScheduleDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          setSelectedSchedule(null);
        }}
        schedule={selectedSchedule}
        backups={backups}
        onSubmit={handleSubmit}
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

export default Schedules;
