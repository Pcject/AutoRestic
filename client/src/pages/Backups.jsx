import React, { useEffect, useState } from 'react';
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Grid,
  IconButton,
  Typography,
  Chip,
  Alert,
  Snackbar,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  useTheme,
  useMediaQuery,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  FormControlLabel,
  Checkbox,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import { ExpandMore as ExpandMoreIcon } from '@mui/icons-material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  PlayArrow as RunIcon,
  Backup as BackupIcon,
} from '@mui/icons-material';
import { backupAPI, repositoryAPI, taskAPI } from '../api';

const BackupDialog = ({ open, onClose, backup, repositories, onSubmit }) => {
  const [formData, setFormData] = useState({
    name: '',
    repository_id: '',
    source_paths: '',
    exclude_patterns: '',
    tags: '',
    options: {
      compression: 'auto'
    },
  });

  useEffect(() => {
    if (backup) {
      setFormData({
        name: backup.name,
        repository_id: backup.repository_id,
        source_paths: backup.source_paths.join('\n'),
        exclude_patterns: backup.exclude_patterns ? backup.exclude_patterns.join('\n') : '',
        tags: backup.tags ? backup.tags.join(',') : '',
        options: {
          compression: 'auto',
          ...(backup.options || {})
        },
      });
    } else {
      setFormData({
        name: '',
        repository_id: '',
        source_paths: '',
        exclude_patterns: '',
        tags: '',
        options: {
          compression: 'auto'
        },
      });
    }
  }, [backup, open]);

  const handleSubmit = (e) => {
    e.preventDefault();
    onSubmit({
      ...formData,
      source_paths: formData.source_paths.split('\n').filter(p => p.trim()),
      exclude_patterns: formData.exclude_patterns ? formData.exclude_patterns.split('\n').filter(p => p.trim()) : undefined,
      tags: formData.tags ? formData.tags.split(',').map(t => t.trim()).filter(t => t) : undefined,
    });
  };

  const updateOption = (key, value) => {
    setFormData({
      ...formData,
      options: {
        ...formData.options,
        [key]: value
      }
    });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ 
        pb: 1,
        background: (theme) => theme.palette.mode === 'light' 
          ? 'linear-gradient(135deg, #10B981 0%, #059669 100%)'
          : 'linear-gradient(135deg, #166534 0%, #065F46 100%)',
        color: '#fff',
        mx: -3,
        mt: -2,
        px: 3,
        py: 2
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <BackupIcon />
          {backup ? '编辑备份' : '新建备份'}
        </Box>
      </DialogTitle>
      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ pt: 3 }}>
          <Grid container spacing={2.5}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="备份名称"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                required
                variant="outlined"
                placeholder="例如：每日文档备份"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                select
                label="选择仓库"
                value={formData.repository_id}
                onChange={(e) => setFormData({ ...formData, repository_id: e.target.value })}
                SelectProps={{ native: true }}
                required
                variant="outlined"
              >
                <option value="">请选择仓库</option>
                {repositories.map((repo) => (
                  <option key={repo.id} value={repo.id}>
                    {repo.name} ({repo.type})
                  </option>
                ))}
              </TextField>
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="源路径 (每行一个)"
                multiline
                rows={4}
                value={formData.source_paths}
                onChange={(e) => setFormData({ ...formData, source_paths: e.target.value })}
                placeholder="/path/to/backup1\n/path/to/backup2"
                required
                helperText="每行输入一个需要备份的路径"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="排除模式 (每行一个，可选)"
                multiline
                rows={3}
                value={formData.exclude_patterns}
                onChange={(e) => setFormData({ ...formData, exclude_patterns: e.target.value })}
                placeholder="*.tmp\nnode_modules\n*.log"
                helperText="支持glob模式的排除规则"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="标签 (逗号分隔，可选)"
                value={formData.tags}
                onChange={(e) => setFormData({ ...formData, tags: e.target.value })}
                placeholder="daily,important"
                helperText="为备份添加标签，便于后续管理"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12}>
              <Accordion>
                <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                  <Typography sx={{ fontWeight: 500 }}>高级选项</Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Grid container spacing={2}>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.dryRun || false}
                            onChange={(e) => updateOption('dryRun', e.target.checked)}
                          />
                        }
                        label="试运行"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.excludeCaches || false}
                            onChange={(e) => updateOption('excludeCaches', e.target.checked)}
                          />
                        }
                        label="排除缓存目录"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.force || false}
                            onChange={(e) => updateOption('force', e.target.checked)}
                          />
                        }
                        label="强制重扫描"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.ignoreCtime || false}
                            onChange={(e) => updateOption('ignoreCtime', e.target.checked)}
                          />
                        }
                        label="忽略 ctime 变化"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.ignoreInode || false}
                            onChange={(e) => updateOption('ignoreInode', e.target.checked)}
                          />
                        }
                        label="忽略 inode 变化"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.noScan || false}
                            onChange={(e) => updateOption('noScan', e.target.checked)}
                          />
                        }
                        label="不扫描大小"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.oneFileSystem || false}
                            onChange={(e) => updateOption('oneFileSystem', e.target.checked)}
                          />
                        }
                        label="单文件系统"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.skipIfUnchanged || false}
                            onChange={(e) => updateOption('skipIfUnchanged', e.target.checked)}
                          />
                        }
                        label="未变化则跳过"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.withAtime || false}
                            onChange={(e) => updateOption('withAtime', e.target.checked)}
                          />
                        }
                        label="保存访问时间"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.noCache || false}
                            onChange={(e) => updateOption('noCache', e.target.checked)}
                          />
                        }
                        label="不使用本地缓存"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.noExtraVerify || false}
                            onChange={(e) => updateOption('noExtraVerify', e.target.checked)}
                          />
                        }
                        label="跳过额外验证"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={formData.options.noLock || false}
                            onChange={(e) => updateOption('noLock', e.target.checked)}
                          />
                        }
                        label="不锁定仓库"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControl fullWidth variant="outlined">
                        <InputLabel>压缩模式</InputLabel>
                        <Select
                          value={formData.options.compression || 'auto'}
                          onChange={(e) => updateOption('compression', e.target.value)}
                          label="压缩模式"
                        >
                          <MenuItem value="auto">自动</MenuItem>
                          <MenuItem value="off">关闭</MenuItem>
                          <MenuItem value="max">最大</MenuItem>
                        </Select>
                      </FormControl>
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="排除大于"
                        value={formData.options.excludeLargerThan || ''}
                        onChange={(e) => updateOption('excludeLargerThan', e.target.value)}
                        placeholder="100M"
                        helperText="例如：100M, 1G"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="主机名"
                        value={formData.options.host || ''}
                        onChange={(e) => updateOption('host', e.target.value)}
                        placeholder="my-host"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="父快照 ID"
                        value={formData.options.parent || ''}
                        onChange={(e) => updateOption('parent', e.target.value)}
                        placeholder="abcdef12"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="读取并发数"
                        type="number"
                        value={formData.options.readConcurrency || ''}
                        onChange={(e) => updateOption('readConcurrency', e.target.value)}
                        placeholder="2"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="限制下载速度 (KiB/s)"
                        type="number"
                        value={formData.options.limitDownload || ''}
                        onChange={(e) => updateOption('limitDownload', e.target.value)}
                        placeholder="1024"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="限制上传速度 (KiB/s)"
                        type="number"
                        value={formData.options.limitUpload || ''}
                        onChange={(e) => updateOption('limitUpload', e.target.value)}
                        placeholder="1024"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <TextField
                        fullWidth
                        label="包大小 (MiB)"
                        type="number"
                        value={formData.options.packSize || ''}
                        onChange={(e) => updateOption('packSize', e.target.value)}
                        placeholder="64"
                        variant="outlined"
                      />
                    </Grid>
                    <Grid item xs={12} sm={6}>
                      <FormControl fullWidth variant="outlined">
                        <InputLabel>详细程度</InputLabel>
                        <Select
                          value={formData.options.verbose !== undefined ? formData.options.verbose.toString() : ''}
                          onChange={(e) => updateOption('verbose', e.target.value === '' ? undefined : parseInt(e.target.value))}
                          label="详细程度"
                        >
                          <MenuItem value="">默认</MenuItem>
                          <MenuItem value="1">详细 (-v)</MenuItem>
                          <MenuItem value="2">非常详细 (-vv)</MenuItem>
                        </Select>
                      </FormControl>
                    </Grid>
                    <Grid item xs={12}>
                      <TextField
                        fullWidth
                        label="时间"
                        value={formData.options.time || ''}
                        onChange={(e) => updateOption('time', e.target.value)}
                        placeholder="2024-01-01 12:00:00"
                        helperText="备份时间戳（可选）"
                        variant="outlined"
                      />
                    </Grid>
                  </Grid>
                </AccordionDetails>
              </Accordion>
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button 
            onClick={onClose}
            variant="outlined"
          >
            取消
          </Button>
          <Button 
            type="submit" 
            variant="contained"
            startIcon={backup ? <EditIcon /> : <AddIcon />}
          >
            {backup ? '更新' : '创建'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};

const Backups = () => {
  const [backups, setBackups] = useState([]);
  const [repositories, setRepositories] = useState([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [selectedBackup, setSelectedBackup] = useState(null);
  const [backupToRun, setBackupToRun] = useState(null);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });
  const [loading, setLoading] = useState(true);
  
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [backupsRes, reposRes] = await Promise.all([
        backupAPI.getAll(),
        repositoryAPI.getAll(),
      ]);
      setBackups(backupsRes.data);
      setRepositories(reposRes.data);
    } catch (error) {
      showSnackbar('获取数据失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleRun = (id) => {
    setBackupToRun(id);
    setConfirmDialogOpen(true);
  };

  const confirmRunBackup = async () => {
    setConfirmDialogOpen(false);
    try {
      await taskAPI.createBackup({ backup_id: backupToRun });
      showSnackbar('备份任务已启动，请查看任务管理页面', 'success');
    } catch (error) {
      showSnackbar('启动备份失败: ' + (error.response?.data?.error || error.message), 'error');
    } finally {
      setBackupToRun(null);
    }
  };

  const handleEdit = (backup) => {
    setSelectedBackup(backup);
    setDialogOpen(true);
  };

  const handleDelete = async (id) => {
    if (window.confirm('确定要删除这个备份吗？')) {
      try {
        await backupAPI.delete(id);
        showSnackbar('备份删除成功', 'success');
        fetchData();
      } catch (error) {
        showSnackbar('备份删除失败', 'error');
      }
    }
  };

  const handleSubmit = async (data) => {
    try {
      if (selectedBackup) {
        await backupAPI.update(selectedBackup.id, data);
        showSnackbar('备份更新成功', 'success');
      } else {
        await backupAPI.create(data);
        showSnackbar('备份创建成功', 'success');
      }
      setDialogOpen(false);
      setSelectedBackup(null);
      fetchData();
    } catch (error) {
      showSnackbar('操作失败: ' + (error.response?.data?.error || error.message), 'error');
    }
  };

  const showSnackbar = (message, severity) => {
    setSnackbar({ open: true, message, severity });
  };

  if (loading) {
    return <Typography>加载中...</Typography>;
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
        <Typography variant="h4" sx={{ fontWeight: 600, fontSize: { xs: '1.5rem', sm: '2.125rem' } }}>备份管理</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => {
            setSelectedBackup(null);
            setDialogOpen(true);
          }}
          fullWidth={isMobile}
        >
          新建备份
        </Button>
      </Box>

      {isMobile ? (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {backups.map((backup) => (
            <Paper key={backup.id} sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                <Box>
                  <Typography variant="h6" sx={{ fontWeight: 600 }}>{backup.name}</Typography>
                  <Typography variant="body2" color="text.secondary">ID: {backup.id}</Typography>
                </Box>
              </Box>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                仓库: {backup.repository_name}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                源路径:
              </Typography>
              {backup.source_paths.slice(0, 2).map((path, idx) => (
                <Typography key={idx} variant="body2" sx={{ wordBreak: 'break-all' }}>
                  • {path}
                </Typography>
              ))}
              {backup.source_paths.length > 2 && (
                <Typography variant="body2" color="text.secondary">
                  +{backup.source_paths.length - 2} more
                </Typography>
              )}
              {backup.tags && backup.tags.length > 0 && (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 2, mb: 2 }}>
                  {backup.tags.map((tag, idx) => (
                    <Chip key={idx} label={tag} size="small" />
                  ))}
                </Box>
              )}
              <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                <IconButton 
                  onClick={() => handleRun(backup.id)} 
                  title="立即备份" 
                  sx={{ color: 'green' }}
                >
                  <RunIcon />
                </IconButton>
                <IconButton 
                  onClick={() => handleEdit(backup)} 
                  title="编辑"
                >
                  <EditIcon />
                </IconButton>
                <IconButton 
                  onClick={() => handleDelete(backup.id)} 
                  title="删除" 
                  sx={{ color: 'red' }}
                >
                  <DeleteIcon />
                </IconButton>
              </Box>
            </Paper>
          ))}
        </Box>
      ) : (
        <TableContainer component={Paper}>
          <Table stickyHeader>
            <TableHead>
              <TableRow>
                <TableCell>ID</TableCell>
                <TableCell>名称</TableCell>
                <TableCell>仓库</TableCell>
                <TableCell>源路径</TableCell>
                <TableCell>标签</TableCell>
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
              {backups.map((backup) => (
                <TableRow key={backup.id}>
                  <TableCell>{backup.id}</TableCell>
                  <TableCell>{backup.name}</TableCell>
                  <TableCell>{backup.repository_name}</TableCell>
                  <TableCell>
                    {backup.source_paths.slice(0, 2).map((path, idx) => (
                      <Typography key={idx} variant="body2" noWrap>
                        {path}
                      </Typography>
                    ))}
                    {backup.source_paths.length > 2 && (
                      <Typography variant="body2" color="text.secondary">
                        +{backup.source_paths.length - 2} more
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>
                    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                      {backup.tags?.map((tag, idx) => (
                        <Chip key={idx} label={tag} size="small" />
                      ))}
                    </Box>
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
                      <IconButton 
                        onClick={() => handleRun(backup.id)} 
                        title="立即备份" 
                        sx={{ color: 'green' }}
                      >
                        <RunIcon />
                      </IconButton>
                      <IconButton 
                        onClick={() => handleEdit(backup)} 
                        title="编辑"
                      >
                        <EditIcon />
                      </IconButton>
                      <IconButton 
                        onClick={() => handleDelete(backup.id)} 
                        title="删除" 
                        sx={{ color: 'red' }}
                      >
                        <DeleteIcon />
                      </IconButton>
                    </Box>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <BackupDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          setSelectedBackup(null);
        }}
        backup={selectedBackup}
        repositories={repositories}
        onSubmit={handleSubmit}
      />

      <Dialog
        open={confirmDialogOpen}
        onClose={() => {
          setConfirmDialogOpen(false);
          setBackupToRun(null);
        }}
      >
        <DialogTitle>确认执行备份</DialogTitle>
        <DialogContent>
          <Typography>确定要立即执行备份吗？</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setConfirmDialogOpen(false);
            setBackupToRun(null);
          }}>
            取消
          </Button>
          <Button onClick={confirmRunBackup} variant="contained" color="primary">
            确认执行
          </Button>
        </DialogActions>
      </Dialog>

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

export default Backups;