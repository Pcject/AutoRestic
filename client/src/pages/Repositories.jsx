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
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  CircularProgress,
  FormControlLabel,
  FormControl,
  FormGroup,
  Checkbox,
  TablePagination,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Divider,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  useTheme,
  useMediaQuery,
  List,
  ListItem,
  ListItemButton,
  Tooltip,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Storage as StorageIcon,
  CheckCircle as CheckIcon,
  Error as ErrorIcon,
  Refresh as RefreshIcon,
  Backup as BackupIcon,
  Restore as RestoreIcon,
  MoreVert as MoreVertIcon,
  Check as CheckRepositoryIcon,
  DeleteSweep as PruneIcon,
  LockOpen as UnlockIcon,
  BarChart as StatsIcon,
  Info as InfoIcon,
  ExpandMore as ExpandMoreIcon,
  FolderOpen as FolderOpenIcon,
  TrendingUp as TrendingUpIcon,
  Folder as FolderIcon,
  InsertDriveFile as FileIcon,
  ArrowBack as ArrowBackIcon,
} from '@mui/icons-material';
import { repositoryAPI, taskAPI } from '../api';

const getPlaceholderForType = (type) => {
  switch (type) {
    case 'local':
      return '/path/to/backup/repo';
    case 'sftp':
      return 'sftp:user@host:/path/to/repo';
    case 's3':
      return 's3:bucket-name/path/to/repo';
    case 'b2':
      return 'b2:bucket-name/path/to/repo';
    case 'azure':
      return 'azure:container-name/path/to/repo';
    case 'gs':
      return 'gs:bucket-name/path/to/repo';
    case 'rest':
      return 'rest:https://user:password@host:8000/path/to/repo';
    default:
      return '/path/to/repo';
  }
};

const getHelperTextForType = (type) => {
  switch (type) {
    case 'local':
      return '本地文件系统路径';
    case 'sftp':
      return 'SFTP服务器路径';
    case 's3':
      return 'Amazon S3兼容存储';
    case 'b2':
      return 'Backblaze B2云存储';
    case 'azure':
      return 'Microsoft Azure Blob存储';
    case 'gs':
      return 'Google Cloud Storage';
    case 'rest':
      return 'REST服务器地址';
    default:
      return '仓库路径';
  }
};

const getEnvVarsConfigForType = (type) => {
  switch (type) {
    case 's3':
      return [
        { key: 'AWS_ACCESS_KEY_ID', label: 'Access Key ID', placeholder: 'your-access-key' },
        { key: 'AWS_SECRET_ACCESS_KEY', label: 'Secret Access Key', placeholder: 'your-secret-key' }
      ];
    case 'b2':
      return [
        { key: 'B2_ACCOUNT_ID', label: 'Account ID', placeholder: 'your-account-id' },
        { key: 'B2_ACCOUNT_KEY', label: 'Account Key', placeholder: 'your-account-key' }
      ];
    case 'azure':
      return [
        { key: 'AZURE_ACCOUNT_NAME', label: 'Account Name', placeholder: 'your-account-name' },
        { key: 'AZURE_ACCOUNT_KEY', label: 'Account Key', placeholder: 'your-account-key' }
      ];
    case 'gs':
      return [
        { key: 'GOOGLE_PROJECT_ID', label: 'Project ID', placeholder: 'your-project-id' },
        { key: 'GOOGLE_APPLICATION_CREDENTIALS', label: 'Credentials Path', placeholder: '/path/to/credentials.json' }
      ];
    case 'rest':
      return [
        { key: 'RESTIC_REST_USERNAME', label: 'Username', placeholder: 'username' },
        { key: 'RESTIC_REST_PASSWORD', label: 'Password', placeholder: 'password' }
      ];
    default:
      return [];
  }
};

const RepositoryDialog = ({ open, onClose, repository, onSubmit }) => {
  const [formData, setFormData] = useState({
    name: '',
    type: 'local',
    url: '',
    password: '',
    env_vars: {},
    auto_init: false,
  });

  useEffect(() => {
    if (repository) {
      setFormData({
        name: repository.name,
        type: repository.type,
        url: repository.url,
        password: repository.password,
        env_vars: repository.env_vars || {},
        auto_init: false,
      });
    } else {
      setFormData({
        name: '',
        type: 'local',
        url: '',
        password: '',
        env_vars: {},
        auto_init: false,
      });
    }
  }, [repository, open]);

  const handleSubmit = (e) => {
    e.preventDefault();
    const envVarsObj = formData.env_vars || {};
    onSubmit({
      ...formData,
      env_vars: Object.keys(envVarsObj).length > 0 ? envVarsObj : undefined,
    });
  };

  const envVarsConfig = getEnvVarsConfigForType(formData.type);
  const showEnvVars = envVarsConfig.length > 0;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
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
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <StorageIcon />
          {repository ? '编辑仓库' : '新建仓库'}
        </Box>
      </DialogTitle>
      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ pt: 3 }}>
          <Grid container spacing={2.5}>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="仓库名称"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                required
                variant="outlined"
                placeholder="例如：我的备份仓库"
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                select
                label="仓库类型"
                value={formData.type}
                onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                SelectProps={{ native: true }}
                required
                variant="outlined"
              >
                <option value="local">📁 本地存储</option>
                <option value="sftp">🔐 SFTP</option>
                <option value="s3">☁️ Amazon S3</option>
                <option value="b2">💾 Backblaze B2</option>
                <option value="azure">🔷 Microsoft Azure</option>
                <option value="gs">🔶 Google Cloud Storage</option>
                <option value="rest">🌐 REST Server</option>
              </TextField>
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="仓库路径"
                value={formData.url}
                onChange={(e) => setFormData({ ...formData, url: e.target.value })}
                placeholder={getPlaceholderForType(formData.type)}
                required
                variant="outlined"
                helperText={getHelperTextForType(formData.type)}
              />
            </Grid>
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="仓库密码"
                type="password"
                value={formData.password}
                onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                required
                variant="outlined"
                placeholder="用于加密备份的密码"
              />
            </Grid>
            {showEnvVars && (
              <Grid item xs={12}>
                <Box sx={{ mb: 1 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    认证配置
                  </Typography>
                </Box>
                <Grid container spacing={2}>
                  {envVarsConfig.map((config) => (
                    <Grid item xs={12} key={config.key}>
                      <TextField
                        fullWidth
                        label={config.label}
                        value={formData.env_vars[config.key] || ''}
                        onChange={(e) => setFormData({
                          ...formData,
                          env_vars: {
                            ...formData.env_vars,
                            [config.key]: e.target.value
                          }
                        })}
                        placeholder={config.placeholder}
                        variant="outlined"
                        type={config.key.includes('SECRET') || config.key.includes('KEY') || config.key.includes('PASSWORD') ? 'password' : 'text'}
                      />
                    </Grid>
                  ))}
                </Grid>
              </Grid>
            )}
            {!repository && (
              <Grid item xs={12}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.auto_init}
                      onChange={(e) => setFormData({ ...formData, auto_init: e.target.checked })}
                      color="primary"
                    />
                  }
                  label="立即初始化仓库"
                />
              </Grid>
            )}
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
            startIcon={repository ? <EditIcon /> : <AddIcon />}
          >
            {repository ? '更新' : '创建'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};

const formatSize = (bytes) => {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
};

const SnapshotsDialog = ({ open, onClose, repository, onRestore, onBrowseFiles }) => {
  const [snapshots, setSnapshots] = useState([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open && repository) {
      fetchSnapshots();
    }
  }, [open, repository]);

  const fetchSnapshots = async () => {
    setLoading(true);
    try {
      const response = await repositoryAPI.getSnapshots(repository.id);
      setSnapshots(response.data);
    } catch (error) {
      console.error('Failed to fetch snapshots:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleBrowseWithPath = (snapshot) => {
    const defaultPath = '/';
    console.log('Opening snapshot path:', defaultPath);
    onBrowseFiles(snapshot.id, defaultPath);
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>
        快照列表 - {repository?.name}
        <IconButton onClick={fetchSnapshots} sx={{ float: 'right' }}>
          <RefreshIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress />
          </Box>
        ) : (
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>快照ID</TableCell>
                  <TableCell>时间</TableCell>
                  <TableCell>主机名</TableCell>
                  <TableCell>路径</TableCell>
                  <TableCell>大小</TableCell>
                  <TableCell>操作</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {snapshots.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} align="center">
                      暂无快照
                    </TableCell>
                  </TableRow>
                ) : (
                  snapshots.map((snapshot) => (
                    <TableRow key={snapshot.id}>
                      <TableCell sx={{ fontFamily: 'monospace' }}>
                        {snapshot.short_id || snapshot.id.substring(0, 8)}
                      </TableCell>
                      <TableCell>
                        {new Date(snapshot.time).toLocaleString()}
                      </TableCell>
                      <TableCell>{snapshot.hostname}</TableCell>
                      <TableCell>
                        {snapshot.paths?.map((p, i) => (
                          <Chip key={i} label={p} size="small" sx={{ mr: 0.5, mb: 0.5 }} />
                        ))}
                      </TableCell>
                      <TableCell>
                        {formatSize(snapshot.size)}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={<FolderOpenIcon />}
                          onClick={() => handleBrowseWithPath(snapshot)}
                          sx={{ mr: 1 }}
                        >
                          浏览
                        </Button>
                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<RestoreIcon />}
                          onClick={() => onRestore(snapshot.id)}
                        >
                          恢复
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>关闭</Button>
      </DialogActions>
    </Dialog>
  );
};

const FileBrowserDialog = ({ open, onClose, repository, snapshotId, initialPath = '/', onRestoreFile }) => {
  const [files, setFiles] = useState([]);
  const [currentPath, setCurrentPath] = useState('/');
  const [loading, setLoading] = useState(false);
  const [selectedFile, setSelectedFile] = useState(null);
  const [restorePath, setRestorePath] = useState('');

  useEffect(() => {
    if (open && repository && snapshotId) {
      const startPath = initialPath || '/';
      setCurrentPath(startPath);
      setSelectedFile(null);
      setRestorePath('');
      fetchFiles(startPath);
    }
  }, [open, repository, snapshotId, initialPath]);

  const fetchFiles = async (path) => {
    setLoading(true);
    try {
      console.log('Fetching files for path:', path);
      const response = await repositoryAPI.ls(repository.id, snapshotId, path);
      const fileData = response.data;
      console.log('Files response:', fileData);
      
      let filesToSet = [];
      if (Array.isArray(fileData)) {
        filesToSet = fileData;
      }
      
      if (filesToSet.length === 0 && path !== '/') {
        console.log('Path is empty, trying root path');
        const rootResponse = await repositoryAPI.ls(repository.id, snapshotId, '/');
        const rootData = rootResponse.data;
        if (Array.isArray(rootData)) {
          filesToSet = rootData;
          setCurrentPath('/');
        }
      }
      
      setFiles(filesToSet);
    } catch (error) {
      console.error('Failed to fetch files:', error);
      setFiles([]);
    } finally {
      setLoading(false);
    }
  };

  const navigateTo = (path) => {
    setCurrentPath(path);
    setSelectedFile(null);
    fetchFiles(path);
  };

  const goBack = () => {
    if (currentPath === '/') return;
    const parentPath = currentPath.split('/').slice(0, -1).join('/') || '/';
    navigateTo(parentPath);
  };

  const formatFileSize = (bytes) => {
    if (!bytes || bytes === 0) return '-';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
  };

  const handleRestore = async () => {
    if (!selectedFile || !restorePath) return;
    try {
      await onRestoreFile(snapshotId, restorePath, selectedFile.path);
      onClose();
    } catch (error) {
      console.error('Failed to restore file:', error);
    }
  };

  const sortedFiles = Array.isArray(files) ? [...files].sort((a, b) => {
    if (a.type === 'dir' && b.type !== 'dir') return -1;
    if (a.type !== 'dir' && b.type === 'dir') return 1;
    return (a.name || '').localeCompare(b.name || '');
  }) : [];

  const displayFiles = currentPath !== '/' 
    ? [{ name: '..', type: 'dir', path: '..', isParent: true }, ...sortedFiles]
    : sortedFiles;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>
        文件浏览器 - {repository?.name}
        <Box sx={{ float: 'right' }}>
          <IconButton onClick={() => fetchFiles(currentPath)}>
            <RefreshIcon />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <IconButton onClick={goBack} disabled={currentPath === '/'}>
            <ArrowBackIcon />
          </IconButton>
          <TextField
            fullWidth
            label="当前路径"
            value={currentPath}
            size="small"
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                navigateTo(e.target.value);
              }
            }}
            onChange={(e) => setCurrentPath(e.target.value)}
            InputProps={{
              endAdornment: (
                <IconButton 
                  size="small" 
                  onClick={() => navigateTo(currentPath)}
                  title="前往"
                >
                  <RefreshIcon />
                </IconButton>
              )
            }}
          />
        </Box>

          {loading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
              <CircularProgress />
            </Box>
          ) : (
            <TableContainer component={Paper} sx={{ maxHeight: 400 }}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell width={50}></TableCell>
                    <TableCell>名称</TableCell>
                    <TableCell width={120}>类型</TableCell>
                    <TableCell width={120}>大小</TableCell>
                    <TableCell width={180}>修改时间</TableCell>
                    <TableCell width={100}>操作</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {displayFiles.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={6} align="center">
                        目录为空
                      </TableCell>
                    </TableRow>
                  ) : (
                    displayFiles.map((file, index) => (
                      <TableRow 
                        key={index}
                        selected={!file.isParent && selectedFile?.path === file.path}
                        hover
                      >
                        <TableCell>
                          <ListItemIcon sx={{ minWidth: 36 }}>
                            {file.isParent ? (
                              <ArrowBackIcon color="action" />
                            ) : file.type === 'dir' ? (
                              <FolderIcon color="primary" />
                            ) : (
                              <FileIcon color="action" />
                            )}
                          </ListItemIcon>
                        </TableCell>
                        <TableCell>
                          {file.name}
                        </TableCell>
                        <TableCell>
                          {file.isParent ? '上一级' : (file.type === 'dir' ? '目录' : '文件')}
                        </TableCell>
                        <TableCell>
                          {file.isParent ? '-' : formatFileSize(file.size)}
                        </TableCell>
                        <TableCell>
                          {file.isParent ? '-' : (file.mtime ? new Date(file.mtime).toLocaleString() : '-')}
                        </TableCell>
                        <TableCell>
                          {file.isParent ? (
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={goBack}
                            >
                              返回
                            </Button>
                          ) : file.type === 'dir' ? (
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => navigateTo(file.path)}
                            >
                              进入
                            </Button>
                          ) : (
                            <Button
                              size="small"
                              variant="contained"
                              onClick={() => {
                                setSelectedFile(file);
                                setRestorePath('');
                              }}
                            >
                              恢复
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>

        {selectedFile && (
          <Paper sx={{ p: 2, mt: 2 }}>
            <Typography variant="subtitle2" gutterBottom>
              恢复文件: {selectedFile.name}
            </Typography>
            <Grid container spacing={2} alignItems="center">
              <Grid item xs={12} sm={8}>
                <TextField
                  fullWidth
                  label="恢复到路径"
                  value={restorePath}
                  onChange={(e) => setRestorePath(e.target.value)}
                  placeholder="/path/to/restore/location"
                  required
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <Button
                  fullWidth
                  variant="contained"
                  startIcon={<RestoreIcon />}
                  onClick={handleRestore}
                  disabled={!restorePath}
                >
                  确认恢复
                </Button>
              </Grid>
            </Grid>
          </Paper>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>关闭</Button>
      </DialogActions>
    </Dialog>
  );
};

const RestoreDialog = ({ open, onClose, repository, snapshotId, onConfirm }) => {
  const [target, setTarget] = useState('');

  useEffect(() => {
    if (open) {
      setTarget('');
    }
  }, [open]);

  const handleConfirm = async () => {
    try {
      await onConfirm(snapshotId, target);
      onClose();
    } catch (error) {
      console.error('Restore failed:', error);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>恢复快照</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          快照ID: <code>{snapshotId}</code>
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          恢复任务将在后台执行，请稍后在任务管理页面查看进度。
        </Typography>
        <TextField
          fullWidth
          label="目标路径"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder="/path/to/restore"
          required
          helperText="请输入要恢复到的本地路径"
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>取消</Button>
        <Button
          variant="contained"
          onClick={handleConfirm}
          disabled={!target}
        >
          确认恢复
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const ForgetDialog = ({ open, onClose, repository, onForget }) => {
  const [formData, setFormData] = useState({
    keepLast: '',
    keepHourly: '',
    keepDaily: '',
    keepWeekly: '',
    keepMonthly: '',
    keepYearly: '',
    keepWithin: '',
    prune: false,
    dryRun: true,
  });
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!window.confirm(formData.prune ? '确定要清理快照并删除未使用的数据吗？这将永久删除数据！' : '确定要清理快照吗？')) {
      return;
    }
    
    setLoading(true);
    try {
      const options = {};
      if (formData.keepLast) options.keepLast = parseInt(formData.keepLast);
      if (formData.keepHourly) options.keepHourly = parseInt(formData.keepHourly);
      if (formData.keepDaily) options.keepDaily = parseInt(formData.keepDaily);
      if (formData.keepWeekly) options.keepWeekly = parseInt(formData.keepWeekly);
      if (formData.keepMonthly) options.keepMonthly = parseInt(formData.keepMonthly);
      if (formData.keepYearly) options.keepYearly = parseInt(formData.keepYearly);
      if (formData.keepWithin) options.keepWithin = formData.keepWithin;
      if (formData.prune) options.prune = true;
      if (formData.dryRun) options.dryRun = true;
      
      await onForget(repository.id, options);
      onClose();
    } catch (error) {
      console.error('Forget failed:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ 
        pb: 1,
        background: (theme) => theme.palette.mode === 'light' 
          ? 'linear-gradient(135deg, #EF4444 0%, #DC2626 100%)'
          : 'linear-gradient(135deg, #7F1D1D 0%, #450A0A 100%)',
        color: '#fff',
        mx: -3,
        mt: -2,
        px: 3,
        py: 2
      }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <DeleteIcon />
          清理快照 - {repository?.name}
        </Box>
      </DialogTitle>
      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ pt: 3 }}>
          <Grid container spacing={2.5}>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留最近N个"
                type="number"
                value={formData.keepLast}
                onChange={(e) => setFormData({ ...formData, keepLast: e.target.value })}
                placeholder="例如: 7"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留每小时N个"
                type="number"
                value={formData.keepHourly}
                onChange={(e) => setFormData({ ...formData, keepHourly: e.target.value })}
                placeholder="例如: 24"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留每天N个"
                type="number"
                value={formData.keepDaily}
                onChange={(e) => setFormData({ ...formData, keepDaily: e.target.value })}
                placeholder="例如: 7"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留每周N个"
                type="number"
                value={formData.keepWeekly}
                onChange={(e) => setFormData({ ...formData, keepWeekly: e.target.value })}
                placeholder="例如: 4"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留每月N个"
                type="number"
                value={formData.keepMonthly}
                onChange={(e) => setFormData({ ...formData, keepMonthly: e.target.value })}
                placeholder="例如: 12"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6} md={4}>
              <TextField
                fullWidth
                label="保留每年N个"
                type="number"
                value={formData.keepYearly}
                onChange={(e) => setFormData({ ...formData, keepYearly: e.target.value })}
                placeholder="例如: 2"
                variant="outlined"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                label="保留最近时间段"
                value={formData.keepWithin}
                onChange={(e) => setFormData({ ...formData, keepWithin: e.target.value })}
                placeholder="例如: 1y2m3d"
                variant="outlined"
                helperText="格式: y(年), m(月), d(日), h(小时)"
              />
            </Grid>
            <Grid item xs={12}>
              <FormGroup>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.dryRun}
                      onChange={(e) => setFormData({ ...formData, dryRun: e.target.checked })}
                    />
                  }
                  label="试运行 (不实际删除)"
                />
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.prune}
                      onChange={(e) => setFormData({ ...formData, prune: e.target.checked })}
                      disabled={formData.dryRun}
                    />
                  }
                  label="同时清理未使用的数据 (prune)"
                />
              </FormGroup>
            </Grid>
          </Grid>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button onClick={onClose} variant="outlined">
            取消
          </Button>
          <Button 
            type="submit" 
            variant="contained" 
            color="error"
            disabled={loading}
            startIcon={loading ? <CircularProgress size={20} /> : <DeleteIcon />}
          >
            {loading ? '处理中...' : '执行清理'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};

const StatsDialog = ({ open, onClose, repository }) => {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open && repository) {
      fetchStats();
    }
  }, [open, repository]);

  const fetchStats = async () => {
    setLoading(true);
    try {
      const response = await repositoryAPI.getStats(repository.id);
      setStats(response.data);
    } catch (error) {
      console.error('Failed to fetch stats:', error);
    } finally {
      setLoading(false);
    }
  };

  const formatSize = (bytes) => {
    if (!bytes || bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
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
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <StorageIcon />
            仓库统计 - {repository?.name}
          </Box>
          <IconButton onClick={fetchStats} sx={{ color: '#fff' }}>
            <RefreshIcon />
          </IconButton>
        </Box>
      </DialogTitle>
      <DialogContent sx={{ pt: 3 }}>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress size={48} />
          </Box>
        ) : stats ? (
          <Box>
            <Grid container spacing={2.5} sx={{ mb: 3 }}>
              <Grid item xs={12} sm={4}>
                <Paper sx={{ p: 2.5, height: '100%' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>总快照数</Typography>
                    <StorageIcon sx={{ fontSize: 20, color: '#5C6BC0' }} />
                  </Box>
                  <Typography variant="h3" sx={{ fontWeight: 700 }}>{stats.total_snapshots || 0}</Typography>
                </Paper>
              </Grid>
              <Grid item xs={12} sm={4}>
                <Paper sx={{ p: 2.5, height: '100%' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>原始大小</Typography>
                    <TrendingUpIcon sx={{ fontSize: 20, color: '#F59E0B' }} />
                  </Box>
                  <Typography variant="h3" sx={{ fontWeight: 700 }}>{formatSize(stats.total_size)}</Typography>
                </Paper>
              </Grid>
              <Grid item xs={12} sm={4}>
                <Paper sx={{ p: 2.5, height: '100%' }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                    <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>压缩后大小</Typography>
                    <StorageIcon sx={{ fontSize: 20, color: '#EF4444' }} />
                  </Box>
                  <Typography variant="h3" sx={{ fontWeight: 700 }}>{formatSize(stats.total_size_compressed)}</Typography>
                </Paper>
              </Grid>
            </Grid>

            <Accordion>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <InfoIcon sx={{ mr: 1, color: 'text.secondary' }} />
                <Typography sx={{ fontWeight: 500 }}>原始数据</Typography>
              </AccordionSummary>
              <AccordionDetails>
                <Paper sx={{ p: 2, backgroundColor: (theme) => theme.palette.mode === 'light' ? '#f5f5f5' : '#1e293b' }}>
                  <pre style={{ 
                    fontSize: '0.75rem', 
                    overflow: 'auto', 
                    margin: 0,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all'
                  }}>
                    {JSON.stringify(stats, null, 2)}
                  </pre>
                </Paper>
              </AccordionDetails>
            </Accordion>
          </Box>
        ) : (
          <Typography color="text.secondary" align="center" sx={{ py: 8 }}>
            无法获取统计信息
          </Typography>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 3 }}>
        <Button onClick={onClose} variant="outlined">
          关闭
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const Repositories = () => {
  const [repositories, setRepositories] = useState([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [snapshotsDialogOpen, setSnapshotsDialogOpen] = useState(false);
  const [restoreDialogOpen, setRestoreDialogOpen] = useState(false);
  const [fileBrowserDialogOpen, setFileBrowserDialogOpen] = useState(false);
  const [statsDialogOpen, setStatsDialogOpen] = useState(false);
  const [forgetDialogOpen, setForgetDialogOpen] = useState(false);
  const [selectedRepo, setSelectedRepo] = useState(null);
  const [selectedSnapshotId, setSelectedSnapshotId] = useState(null);
  const [fileBrowserSnapshotId, setFileBrowserSnapshotId] = useState(null);
  const [fileBrowserInitialPath, setFileBrowserInitialPath] = useState('/');
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [anchorEl, setAnchorEl] = useState(null);
  const [actionMenuRepo, setActionMenuRepo] = useState(null);
  const [operationLoading, setOperationLoading] = useState({});
  
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('sm'));

  useEffect(() => {
    fetchRepositories();
  }, []);

  const fetchRepositories = async () => {
    try {
      const response = await repositoryAPI.getAll();
      setRepositories(response.data);
    } catch (error) {
      showSnackbar('获取仓库列表失败', 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleMenuOpen = (event, repo) => {
    setAnchorEl(event.currentTarget);
    setActionMenuRepo(repo);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setActionMenuRepo(null);
  };

  const setLoadingState = (repoId, isLoading) => {
    setOperationLoading(prev => ({ ...prev, [repoId]: isLoading }));
  };

  const handleInit = async (repo) => {
    handleMenuClose();
    setLoadingState(repo.id, true);
    try {
      await taskAPI.createInit({ repository_id: repo.id });
      showSnackbar('初始化任务已启动，请查看任务管理页面', 'success');
      fetchRepositories();
    } catch (error) {
      showSnackbar('启动初始化任务失败: ' + (error.response?.data?.error || error.message), 'error');
    } finally {
      setLoadingState(repo.id, false);
    }
  };

  const handleCheckInit = async (repo) => {
    handleMenuClose();
    setLoadingState(repo.id, true);
    try {
      await repositoryAPI.checkInit(repo.id);
      showSnackbar('检查仓库状态成功', 'success');
      fetchRepositories();
    } catch (error) {
      showSnackbar('检查仓库状态失败', 'error');
    } finally {
      setLoadingState(repo.id, false);
    }
  };

  const handleCheck = async (repo) => {
    handleMenuClose();
    setLoadingState(repo.id, true);
    try {
      await repositoryAPI.check(repo.id);
      showSnackbar('仓库检查成功', 'success');
    } catch (error) {
      showSnackbar('仓库检查失败: ' + (error.response?.data?.error || error.message), 'error');
    } finally {
      setLoadingState(repo.id, false);
    }
  };

  const handlePrune = async (repo) => {
    handleMenuClose();
    if (window.confirm('确定要清理仓库吗？这将删除未使用的数据。')) {
      setLoadingState(repo.id, true);
      try {
        await repositoryAPI.prune(repo.id);
        showSnackbar('仓库清理成功', 'success');
      } catch (error) {
        showSnackbar('仓库清理失败: ' + (error.response?.data?.error || error.message), 'error');
      } finally {
        setLoadingState(repo.id, false);
      }
    }
  };

  const handleUnlock = async (repo) => {
    handleMenuClose();
    if (window.confirm('确定要解锁仓库吗？这将移除所有锁。')) {
      setLoadingState(repo.id, true);
      try {
        await repositoryAPI.unlock(repo.id);
        showSnackbar('仓库解锁成功', 'success');
      } catch (error) {
        showSnackbar('仓库解锁失败: ' + (error.response?.data?.error || error.message), 'error');
      } finally {
        setLoadingState(repo.id, false);
      }
    }
  };

  const handleEdit = (repo) => {
    handleMenuClose();
    setSelectedRepo(repo);
    setDialogOpen(true);
  };

  const handleDelete = async (repo) => {
    handleMenuClose();
    if (window.confirm('确定要删除这个仓库吗？')) {
      setLoadingState(repo.id, true);
      try {
        await repositoryAPI.delete(repo.id);
        showSnackbar('仓库删除成功', 'success');
        fetchRepositories();
      } catch (error) {
        showSnackbar('仓库删除失败', 'error');
      } finally {
        setLoadingState(repo.id, false);
      }
    }
  };

  const handleViewSnapshots = (repo) => {
    handleMenuClose();
    setSelectedRepo(repo);
    setSnapshotsDialogOpen(true);
  };

  const handleViewStats = (repo) => {
    handleMenuClose();
    setSelectedRepo(repo);
    setStatsDialogOpen(true);
  };

  const handleForget = (repo) => {
    handleMenuClose();
    setSelectedRepo(repo);
    setForgetDialogOpen(true);
  };

  const handleExecuteForget = async (repoId, options) => {
    try {
      await repositoryAPI.forget(repoId, options);
      showSnackbar('快照清理成功', 'success');
      fetchRepositories();
    } catch (error) {
      showSnackbar('快照清理失败: ' + (error.response?.data?.error || error.message), 'error');
      throw error;
    }
  };

  const handleBrowseFiles = (snapshotId, initialPath = '/') => {
    setFileBrowserSnapshotId(snapshotId);
    setFileBrowserInitialPath(initialPath);
    setFileBrowserDialogOpen(true);
  };

  const handleRestore = (snapshotId) => {
    setSelectedSnapshotId(snapshotId);
    setSnapshotsDialogOpen(false);
    setRestoreDialogOpen(true);
  };

  const handleConfirmRestore = async (snapshotId, target) => {
    try {
      await taskAPI.createRestore({ 
        repository_id: selectedRepo.id,
        snapshot_id: snapshotId, 
        target 
      });
      showSnackbar('恢复任务已启动，请查看任务管理页面', 'success');
    } catch (error) {
      showSnackbar('启动恢复任务失败: ' + (error.response?.data?.error || error.message), 'error');
      throw error;
    }
  };

  const handleRestoreFile = async (snapshotId, target, filePath) => {
    try {
      await taskAPI.createRestore({ 
        repository_id: selectedRepo.id,
        snapshot_id: snapshotId, 
        target,
        include: filePath
      });
      showSnackbar('文件恢复任务已启动，请查看任务管理页面', 'success');
    } catch (error) {
      showSnackbar('启动文件恢复任务失败: ' + (error.response?.data?.error || error.message), 'error');
      throw error;
    }
  };

  const handleSubmit = async (data) => {
    try {
      if (selectedRepo) {
        await repositoryAPI.update(selectedRepo.id, data);
        showSnackbar('仓库更新成功', 'success');
      } else {
        await repositoryAPI.create(data);
        showSnackbar('仓库创建成功', 'success');
      }
      setDialogOpen(false);
      setSelectedRepo(null);
      fetchRepositories();
    } catch (error) {
      showSnackbar('操作失败: ' + (error.response?.data?.error || error.message), 'error');
    }
  };

  const showSnackbar = (message, severity) => {
    setSnackbar({ open: true, message, severity });
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
        <Typography variant="h4" sx={{ fontWeight: 600, fontSize: { xs: '1.5rem', sm: '2.125rem' } }}>仓库管理</Typography>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => {
            setSelectedRepo(null);
            setDialogOpen(true);
          }}
          fullWidth={isMobile}
        >
          新建仓库
        </Button>
      </Box>

      {isMobile ? (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {repositories
            .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
            .map((repo) => (
              <Paper key={repo.id} sx={{ p: 2 }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                  <Box>
                    <Typography variant="h6" sx={{ fontWeight: 600 }}>{repo.name}</Typography>
                    <Typography variant="body2" color="text.secondary">ID: {repo.id}</Typography>
                  </Box>
                  <Chip
                    icon={repo.initialized ? <CheckIcon /> : <ErrorIcon />}
                    label={repo.initialized ? '已初始化' : '未初始化'}
                    color={repo.initialized ? 'success' : 'warning'}
                    size="small"
                  />
                </Box>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  类型: {repo.type}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2, wordBreak: 'break-all' }}>
                  URL: {repo.url}
                </Typography>
                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                  {operationLoading[repo.id] ? (
                    <CircularProgress size={24} />
                  ) : (
                    <>
                      <IconButton
                        onClick={(e) => handleMenuOpen(e, repo)}
                        size="small"
                      >
                        <MoreVertIcon />
                      </IconButton>
                      {repo.initialized && (
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={<BackupIcon />}
                          onClick={() => handleViewSnapshots(repo)}
                        >
                          查看快照
                        </Button>
                      )}
                    </>
                  )}
                </Box>
              </Paper>
            ))}
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={repositories.length}
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
                <TableCell>名称</TableCell>
                <TableCell>类型</TableCell>
                <TableCell>URL</TableCell>
                <TableCell>状态</TableCell>
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
              {repositories
                .slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage)
                .map((repo) => (
                  <TableRow key={repo.id}>
                    <TableCell>{repo.id}</TableCell>
                    <TableCell sx={{ fontWeight: 500 }}>{repo.name}</TableCell>
                    <TableCell>{repo.type}</TableCell>
                    <TableCell sx={{ maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {repo.url}
                    </TableCell>
                    <TableCell>
                      <Chip
                        icon={repo.initialized ? <CheckIcon /> : <ErrorIcon />}
                        label={repo.initialized ? '已初始化' : '未初始化'}
                        color={repo.initialized ? 'success' : 'warning'}
                        size="small"
                      />
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
                      {operationLoading[repo.id] ? (
                        <CircularProgress size={24} />
                      ) : (
                        <>
                          <IconButton
                            onClick={(e) => handleMenuOpen(e, repo)}
                            size="small"
                          >
                            <MoreVertIcon />
                          </IconButton>
                          {repo.initialized && (
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={<BackupIcon />}
                              onClick={() => handleViewSnapshots(repo)}
                              sx={{ ml: 1 }}
                            >
                              查看快照
                            </Button>
                          )}
                        </>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
          <TablePagination
            rowsPerPageOptions={[5, 10, 25]}
            component="div"
            count={repositories.length}
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

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        {!actionMenuRepo?.initialized && (
          <MenuItem onClick={() => handleInit(actionMenuRepo)}>
            <ListItemIcon>
              <StorageIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText>初始化仓库</ListItemText>
          </MenuItem>
        )}
        <MenuItem onClick={() => handleCheckInit(actionMenuRepo)}>
          <ListItemIcon>
            <RefreshIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>检查状态</ListItemText>
        </MenuItem>
        {actionMenuRepo?.initialized && (
          <>
            <Divider />
            <MenuItem onClick={() => handleViewSnapshots(actionMenuRepo)}>
              <ListItemIcon>
                <BackupIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>查看快照</ListItemText>
            </MenuItem>
            <MenuItem onClick={() => handleViewStats(actionMenuRepo)}>
              <ListItemIcon>
                <StatsIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>仓库统计</ListItemText>
            </MenuItem>
            <MenuItem onClick={() => handleForget(actionMenuRepo)}>
              <ListItemIcon>
                <DeleteIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>清理快照</ListItemText>
            </MenuItem>
            <Divider />
            <MenuItem onClick={() => handleCheck(actionMenuRepo)}>
              <ListItemIcon>
                <CheckRepositoryIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>检查仓库</ListItemText>
            </MenuItem>
            <MenuItem onClick={() => handlePrune(actionMenuRepo)}>
              <ListItemIcon>
                <PruneIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>清理仓库</ListItemText>
            </MenuItem>
            <MenuItem onClick={() => handleUnlock(actionMenuRepo)}>
              <ListItemIcon>
                <UnlockIcon fontSize="small" />
              </ListItemIcon>
              <ListItemText>解锁仓库</ListItemText>
            </MenuItem>
          </>
        )}
        <Divider />
        <MenuItem onClick={() => handleEdit(actionMenuRepo)}>
          <ListItemIcon>
            <EditIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>编辑</ListItemText>
        </MenuItem>
        <MenuItem onClick={() => handleDelete(actionMenuRepo)} sx={{ color: 'error.main' }}>
          <ListItemIcon sx={{ color: 'error.main' }}>
            <DeleteIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>删除</ListItemText>
        </MenuItem>
      </Menu>

      <RepositoryDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          setSelectedRepo(null);
        }}
        repository={selectedRepo}
        onSubmit={handleSubmit}
      />

      <SnapshotsDialog
        open={snapshotsDialogOpen}
        onClose={() => {
          setSnapshotsDialogOpen(false);
        }}
        repository={selectedRepo}
        onRestore={handleRestore}
        onBrowseFiles={handleBrowseFiles}
      />

      <FileBrowserDialog
        open={fileBrowserDialogOpen}
        onClose={() => {
          setFileBrowserDialogOpen(false);
          setFileBrowserSnapshotId(null);
          setFileBrowserInitialPath('/');
          setSnapshotsDialogOpen(true);
        }}
        repository={selectedRepo}
        snapshotId={fileBrowserSnapshotId}
        initialPath={fileBrowserInitialPath}
        onRestoreFile={handleRestoreFile}
      />

      <RestoreDialog
        open={restoreDialogOpen}
        onClose={() => {
          setRestoreDialogOpen(false);
          setSelectedSnapshotId(null);
        }}
        repository={selectedRepo}
        snapshotId={selectedSnapshotId}
        onConfirm={handleConfirmRestore}
      />

      <StatsDialog
        open={statsDialogOpen}
        onClose={() => {
          setStatsDialogOpen(false);
        }}
        repository={selectedRepo}
      />

      <ForgetDialog
        open={forgetDialogOpen}
        onClose={() => {
          setForgetDialogOpen(false);
        }}
        repository={selectedRepo}
        onForget={handleExecuteForget}
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

export default Repositories;
