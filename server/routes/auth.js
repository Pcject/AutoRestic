const express = require('express');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const { db } = require('../config/database');
const authMiddleware = require('../middleware/auth');
const logger = require('../utils/logger');

const router = express.Router();

router.post('/login', async (req, res) => {
  try {
    const { username, password } = req.body;

    if (!username || !password) {
      return res.status(400).json({ error: 'Username and password are required' });
    }

    const user = db.prepare('SELECT * FROM users WHERE username = ?').get(username);

    if (!user) {
      return res.status(401).json({ error: 'Invalid credentials' });
    }

    const isValidPassword = await bcrypt.compare(password, user.password);

    if (!isValidPassword) {
      return res.status(401).json({ error: 'Invalid credentials' });
    }

    const token = jwt.sign(
      { userId: user.id, username: user.username },
      process.env.JWT_SECRET,
      { expiresIn: '24h' }
    );

    logger.info(`User ${username} logged in successfully`);
    res.json({ token, username: user.username });
  } catch (error) {
    logger.error('Login error:', error);
    res.status(500).json({ error: 'Login failed' });
  }
});

router.post('/register', async (req, res) => {
  try {
    const { username, password } = req.body;

    if (!username || !password) {
      return res.status(400).json({ error: 'Username and password are required' });
    }

    if (password.length < 6) {
      return res.status(400).json({ error: 'Password must be at least 6 characters' });
    }

    const hashedPassword = await bcrypt.hash(password, 10);

    const result = db.prepare(
      'INSERT INTO users (username, password) VALUES (?, ?)'
    ).run(username, hashedPassword);

    logger.info(`User ${username} registered successfully`);
    res.status(201).json({ message: 'User created successfully', userId: result.lastInsertRowid });
  } catch (error) {
    if (error.code === 'SQLITE_CONSTRAINT') {
      return res.status(409).json({ error: 'Username already exists' });
    }
    logger.error('Registration error:', error);
    res.status(500).json({ error: 'Registration failed' });
  }
});

router.get('/me', (req, res) => {
  try {
    const token = req.headers.authorization?.split(' ')[1];
    
    if (!token) {
      return res.status(401).json({ error: 'No token provided' });
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    const user = db.prepare('SELECT id, username, created_at FROM users WHERE id = ?').get(decoded.userId);

    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    res.json(user);
  } catch (error) {
    logger.error('Get user error:', error);
    res.status(401).json({ error: 'Invalid token' });
  }
});

router.put('/me', authMiddleware, async (req, res) => {
  try {
    const token = req.headers.authorization?.split(' ')[1];
    
    if (!token) {
      return res.status(401).json({ error: 'No token provided' });
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    const { username, currentPassword, newPassword } = req.body;
    
    const user = db.prepare('SELECT * FROM users WHERE id = ?').get(decoded.userId);
    
    if (!user) {
      return res.status(404).json({ error: 'User not found' });
    }

    const updates = {};
    
    if (username && username !== user.username) {
      const existingUser = db.prepare('SELECT id FROM users WHERE username = ?').get(username);
      if (existingUser) {
        return res.status(409).json({ error: 'Username already exists' });
      }
      updates.username = username;
    }

    if (newPassword) {
      if (!currentPassword) {
        return res.status(400).json({ error: 'Current password is required to change password' });
      }
      
      const isValidPassword = await bcrypt.compare(currentPassword, user.password);
      if (!isValidPassword) {
        return res.status(401).json({ error: 'Current password is incorrect' });
      }
      
      if (newPassword.length < 6) {
        return res.status(400).json({ error: 'New password must be at least 6 characters' });
      }
      
      updates.password = await bcrypt.hash(newPassword, 10);
    }

    if (Object.keys(updates).length > 0) {
      const setClauses = Object.keys(updates).map(key => `${key} = ?`).join(', ');
      const values = Object.values(updates);
      values.push(decoded.userId);
      
      db.prepare(`UPDATE users SET ${setClauses} WHERE id = ?`).run(...values);
      
      logger.info(`User ${decoded.userId} updated successfully`);
    }

    const updatedUser = db.prepare('SELECT id, username, created_at FROM users WHERE id = ?').get(decoded.userId);
    res.json({ message: 'Profile updated successfully', user: updatedUser });
  } catch (error) {
    logger.error('Update user error:', error);
    res.status(500).json({ error: 'Failed to update profile' });
  }
});

module.exports = router;