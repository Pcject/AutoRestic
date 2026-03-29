import axios from 'axios';

const API_BASE_URL = import.meta.env.VITE_API_URL || '/api';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export const authAPI = {
  login: (username, password) => api.post('/auth/login', { username, password }),
  register: (username, password) => api.post('/auth/register', { username, password }),
  getMe: () => api.get('/auth/me'),
  updateProfile: (data) => api.put('/auth/me', data),
};

export const repositoryAPI = {
  getAll: () => api.get('/repositories'),
  getById: (id) => api.get(`/repositories/${id}`),
  create: (data) => api.post('/repositories', data),
  update: (id, data) => api.put(`/repositories/${id}`, data),
  delete: (id) => api.delete(`/repositories/${id}`),
  init: (id) => api.post(`/repositories/${id}/init`),
  checkInit: (id) => api.post(`/repositories/${id}/check-init`),
  getSnapshots: (id) => api.get(`/repositories/${id}/snapshots`),
  restore: (id, data) => api.post(`/repositories/${id}/restore`, data),
  check: (id, options) => api.post(`/repositories/${id}/check`, options),
  prune: (id) => api.post(`/repositories/${id}/prune`),
  forget: (id, options) => api.post(`/repositories/${id}/forget`, options),
  getStats: (id, options) => api.get(`/repositories/${id}/stats`, { params: options }),
  unlock: (id) => api.post(`/repositories/${id}/unlock`),
  ls: (id, snapshotId, path) => api.get(`/repositories/${id}/ls/${snapshotId}`, { params: { path } }),
};

export const backupAPI = {
  getAll: () => api.get('/backups'),
  getById: (id) => api.get(`/backups/${id}`),
  create: (data) => api.post('/backups', data),
  update: (id, data) => api.put(`/backups/${id}`, data),
  delete: (id) => api.delete(`/backups/${id}`),
  run: (id) => api.post(`/backups/${id}/run`),
  getSnapshots: (id) => api.get(`/backups/${id}/snapshots`),
  getLogs: (id) => api.get(`/backups/${id}/logs`),
};

export const scheduleAPI = {
  getAll: () => api.get('/schedules'),
  getById: (id) => api.get(`/schedules/${id}`),
  create: (data) => api.post('/schedules', data),
  update: (id, data) => api.put(`/schedules/${id}`, data),
  delete: (id) => api.delete(`/schedules/${id}`),
  toggle: (id) => api.post(`/schedules/${id}/toggle`),
};

export const logAPI = {
  getAll: (params) => api.get('/logs', { params }),
  getStats: () => api.get('/logs/stats'),
  getById: (id) => api.get(`/logs/${id}`),
};

export const taskAPI = {
  getAll: (params) => api.get('/tasks', { params }),
  getById: (id) => api.get(`/tasks/${id}`),
  getLogs: (id) => api.get(`/tasks/${id}/logs`),
  createBackup: (data) => api.post('/tasks/backup', data),
  createRestore: (data) => api.post('/tasks/restore', data),
  createCheck: (data) => api.post('/tasks/check', data),
  createPrune: (data) => api.post('/tasks/prune', data),
  createInit: (data) => api.post('/tasks/init', data),
};

export default api;