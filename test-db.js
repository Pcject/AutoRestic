const Database = require('better-sqlite3');
const path = require('path');

const dbDir = path.join(__dirname, 'data');
const dbPath = path.join(dbDir, 'autorestic.db');

console.log('Database path:', dbPath);
console.log('Checking if database file exists:', require('fs').existsSync(dbPath));

const db = new Database(dbPath);

console.log('\nChecking operation_logs table structure:');
const columns = db.prepare('PRAGMA table_info(operation_logs)').all();
console.log('Columns:', columns.map(c => c.name));

console.log('\nChecking if duration column exists:');
const hasDuration = columns.some(c => c.name === 'duration');
console.log('duration column exists:', hasDuration);

db.close();
