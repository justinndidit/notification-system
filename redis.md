# Redis Complete Guide

This guide will teach you everything you need to know about setting up and using Redis in your NestJS applications, based on the patterns used in this notification system.

## Table of Contents

1. [What is Redis?](#what-is-redis)
2. [Installation & Setup](#installation--setup)
3. [Configuration](#configuration)
4. [Creating a Redis Module](#creating-a-redis-module)
5. [Using Redis in Your Services](#using-redis-in-your-services)
6. [Basic Operations](#basic-operations)
7. [Advanced Operations](#advanced-operations)
8. [Caching Patterns](#caching-patterns)
9. [Error Handling](#error-handling)
10. [Best Practices](#best-practices)
11. [Troubleshooting](#troubleshooting)

---

## What is Redis?

Redis (Remote Dictionary Server) is an in-memory data structure store that can be used as:

- **Cache**: Store frequently accessed data for fast retrieval
- **Session Store**: Store user sessions
- **Rate Limiting**: Track API request counts
- **Message Broker**: Pub/sub messaging (though RabbitMQ is better for this)
- **Real-time Analytics**: Store counters and metrics

### Why Use Redis?

1. **Speed**: Data is stored in RAM, making it extremely fast (microsecond latency)
2. **Persistence**: Can optionally save data to disk
3. **Data Structures**: Supports strings, hashes, lists, sets, sorted sets
4. **TTL (Time To Live)**: Automatic expiration of keys
5. **Atomic Operations**: Operations are atomic, preventing race conditions

---

## Installation & Setup

### Option 1: Local Installation (macOS)

```bash
# Using Homebrew
brew install redis

# Start Redis server
brew services start redis

# Or run manually
redis-server
```

### Option 2: Docker (Recommended for Development)

The project already has Redis configured in `infra/docker-compose.local.yaml`:

```yaml
redis:
  image: redis:7.2-alpine
  container_name: redis
  restart: unless-stopped
  command: redis-server --save 20 1 --loglevel warning
  ports:
    - '6379:6379'
  volumes:
    - redis_data:/data
  networks:
    - backend
```

Start it with:

```bash
cd infra
docker-compose -f docker-compose.local.yaml up redis -d
```

### Option 3: Redis Cloud (Production)

1. Sign up at [Redis Cloud](https://redis.com/try-free/)
2. Create a database
3. Get your connection URL (format: `redis://username:password@host:port`)

### Verify Installation

```bash
# Test connection
redis-cli ping
# Should return: PONG

# Or if using Docker
docker exec -it redis redis-cli ping
```

---

## Configuration

### 1. Install Dependencies

In your NestJS service, install `ioredis`:

```bash
npm install ioredis
npm install --save-dev @types/ioredis  # TypeScript types (if needed)
```

### 2. Environment Variables

Add to your `.env` file:

```env
# Local Redis (no password)
REDIS_URL=redis://localhost:6379

# Redis with password
REDIS_URL=redis://:password@localhost:6379

# Redis Cloud format
REDIS_URL=redis://username:password@host:port

# Or separate variables
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=your_password
REDIS_DB=0
```

### 3. Config File

Create or update `src/config/config.ts`:

```typescript
export default () => ({
  port: parseInt(process.env.PORT || '3000', 10),
  redisUrl: process.env.REDIS_URL || 'redis://localhost:6379',
  // Or if using separate variables:
  redis: {
    host: process.env.REDIS_HOST || 'localhost',
    port: parseInt(process.env.REDIS_PORT || '6379', 10),
    password: process.env.REDIS_PASSWORD || undefined,
    db: parseInt(process.env.REDIS_DB || '0', 10),
  },
});
```

---

## Creating a Redis Module

This is the foundation - a reusable module that provides Redis client to your entire application.

### Step 1: Create `src/common/redis.module.ts`

```typescript
import { Module, Global } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import Redis from 'ioredis';
import config from '../config/config';

const { redisUrl } = config();

@Global() // Makes this module available everywhere without importing
@Module({
  imports: [ConfigModule],
  providers: [
    {
      provide: 'REDIS_CLIENT', // This is the token we'll use to inject
      useFactory: () => {
        // Get connection string from config
        const connectionString = redisUrl || 'redis://localhost:6379';

        // Create Redis client with options
        const redisClient = new Redis(connectionString, {
          // Retry strategy: wait longer between retries (max 2 seconds)
          retryStrategy: times => {
            const delay = Math.min(times * 50, 2000);
            return delay;
          },
          // Enable ready check before executing commands
          enableReadyCheck: true,
          // Max retries per request
          maxRetriesPerRequest: 3,
        });

        // Event listeners for monitoring
        redisClient.on('connect', () => {
          console.log('✅ Redis client connected');
        });

        redisClient.on('ready', () => {
          console.log('✅ Redis client ready');
        });

        redisClient.on('error', err => {
          console.error('❌ Redis client error:', err.message);
        });

        redisClient.on('close', () => {
          console.log('⚠️  Redis client connection closed');
        });

        redisClient.on('reconnecting', () => {
          console.log('🔄 Redis client reconnecting...');
        });

        return redisClient;
      },
    },
  ],
  exports: ['REDIS_CLIENT'], // Export so other modules can use it
})
export class RedisModule {}
```

### Step 2: Import in Your App Module

In `src/app.module.ts`:

```typescript
import { Module } from '@nestjs/common';
import { RedisModule } from './common/redis.module';

@Module({
  imports: [
    RedisModule, // Add this
    // ... other modules
  ],
  // ...
})
export class AppModule {}
```

**Why `@Global()`?** It means once you import `RedisModule` in `AppModule`, you can inject `REDIS_CLIENT` in any service without importing `RedisModule` again.

---

## Using Redis in Your Services

### Step 1: Inject Redis Client

```typescript
import { Injectable, Inject } from '@nestjs/common';
import Redis from 'ioredis';

@Injectable()
export class MyService {
  constructor(
    @Inject('REDIS_CLIENT') // Inject using the token
    private readonly redis: Redis // Type is Redis from ioredis
  ) {}
}
```

### Step 2: Use Redis Operations

Now you can use any Redis command:

```typescript
@Injectable()
export class MyService {
  constructor(
    @Inject('REDIS_CLIENT')
    private readonly redis: Redis
  ) {}

  async example() {
    // Set a value
    await this.redis.set('key', 'value');

    // Get a value
    const value = await this.redis.get('key');

    // Delete a key
    await this.redis.del('key');
  }
}
```

---

## Basic Operations

### SET - Store a Value

```typescript
// Simple set (string)
await this.redis.set('user:123', 'John Doe');

// Set with expiration (TTL in seconds)
await this.redis.setex('session:abc', 3600, 'session_data'); // Expires in 1 hour

// Set with expiration (TTL in milliseconds)
await this.redis.psetex('temp:key', 5000, 'data'); // Expires in 5 seconds

// Set only if key doesn't exist (NX = Not eXists)
await this.redis.set('user:123', 'John', 'EX', 3600, 'NX');

// Set only if key exists (XX = eXists)
await this.redis.set('user:123', 'John', 'EX', 3600, 'XX');
```

### GET - Retrieve a Value

```typescript
// Get string value
const value = await this.redis.get('user:123');
// Returns: "John Doe" or null if not found

// Get multiple keys
const values = await this.redis.mget('key1', 'key2', 'key3');
// Returns: ["value1", "value2", "value3"] or null for missing keys
```

### DELETE - Remove Keys

```typescript
// Delete single key
await this.redis.del('user:123');

// Delete multiple keys
await this.redis.del('key1', 'key2', 'key3');

// Returns: number of keys deleted
```

### EXISTS - Check if Key Exists

```typescript
// Check if key exists
const exists = await this.redis.exists('user:123');
// Returns: 1 if exists, 0 if not

// Check multiple keys
const count = await this.redis.exists('key1', 'key2');
// Returns: number of keys that exist
```

### TTL - Time To Live

```typescript
// Get remaining TTL in seconds
const ttl = await this.redis.ttl('user:123');
// Returns: seconds remaining, -1 if no expiration, -2 if key doesn't exist

// Get remaining TTL in milliseconds
const pttl = await this.redis.pttl('user:123');

// Set expiration on existing key
await this.redis.expire('user:123', 3600); // Expire in 1 hour

// Remove expiration (make key persistent)
await this.redis.persist('user:123');
```

---

## Advanced Operations

### Working with JSON Objects

Since Redis stores strings, you need to serialize/deserialize JSON:

```typescript
// Store object
const user = { id: '123', name: 'John', email: 'john@example.com' };
await this.redis.set('user:123', JSON.stringify(user), 'EX', 3600);

// Retrieve object
const data = await this.redis.get('user:123');
const user = data ? JSON.parse(data) : null;
```

### Pattern Matching (KEYS)

```typescript
// Get all keys matching pattern (use carefully in production!)
const keys = await this.redis.keys('user:*');
// Returns: ["user:123", "user:456", "user:789"]

// Better: Use SCAN for large datasets (cursor-based)
let cursor = '0';
const keys: string[] = [];
do {
  const [nextCursor, results] = await this.redis.scan(
    cursor,
    'MATCH',
    'user:*',
    'COUNT',
    100
  );
  cursor = nextCursor;
  keys.push(...results);
} while (cursor !== '0');
```

### Increment/Decrement (Counters)

```typescript
// Increment by 1
const count = await this.redis.incr('page:views');
// Returns: new value (1, 2, 3, ...)

// Increment by specific amount
const count = await this.redis.incrby('page:views', 5);
// Returns: new value

// Decrement by 1
const count = await this.redis.decr('page:views');

// Decrement by specific amount
const count = await this.redis.decrby('page:views', 3);

// Increment floating point
const count = await this.redis.incrbyfloat('score', 1.5);
```

### Hashes (Object-like Storage)

```typescript
// Set hash field
await this.redis.hset('user:123', 'name', 'John');
await this.redis.hset('user:123', 'email', 'john@example.com');

// Set multiple hash fields at once
await this.redis.hset('user:123', {
  name: 'John',
  email: 'john@example.com',
  age: '30',
});

// Get hash field
const name = await this.redis.hget('user:123', 'name');

// Get all hash fields
const user = await this.redis.hgetall('user:123');
// Returns: { name: 'John', email: 'john@example.com', age: '30' }

// Get multiple hash fields
const fields = await this.redis.hmget('user:123', 'name', 'email');

// Delete hash field
await this.redis.hdel('user:123', 'email');

// Check if hash field exists
const exists = await this.redis.hexists('user:123', 'name');

// Get all hash field names
const fieldNames = await this.redis.hkeys('user:123');

// Get all hash values
const values = await this.redis.hvals('user:123');
```

### Lists (Arrays)

```typescript
// Add to end of list
await this.redis.rpush('tasks', 'task1', 'task2', 'task3');

// Add to beginning of list
await this.redis.lpush('tasks', 'task0');

// Get list length
const length = await this.redis.llen('tasks');

// Get list elements
const tasks = await this.redis.lrange('tasks', 0, -1); // Get all
const first = await this.redis.lrange('tasks', 0, 0); // Get first

// Remove and return first element
const task = await this.redis.lpop('tasks');

// Remove and return last element
const task = await this.redis.rpop('tasks');
```

### Sets (Unique Collections)

```typescript
// Add members to set
await this.redis.sadd('tags', 'javascript', 'nodejs', 'redis');

// Get all members
const tags = await this.redis.smembers('tags');

// Check if member exists
const exists = await this.redis.sismember('tags', 'javascript');

// Remove member
await this.redis.srem('tags', 'javascript');

// Get set size
const size = await this.redis.scard('tags');

// Get intersection of sets
const common = await this.redis.sinter('set1', 'set2');

// Get union of sets
const all = await this.redis.sunion('set1', 'set2');
```

---

## Caching Patterns

### Pattern 1: Simple Cache Service

Create a reusable cache service (like in `user-service` and `template-service`):

```typescript
import { Injectable, Inject, Logger } from '@nestjs/common';
import Redis from 'ioredis';

@Injectable()
export class CacheService {
  private readonly logger = new Logger(CacheService.name);
  private readonly DEFAULT_TTL = 600; // 10 minutes

  constructor(@Inject('REDIS_CLIENT') private readonly redis: Redis) {}

  /**
   * Get value from cache
   */
  async get<T>(key: string): Promise<T | null> {
    try {
      const data = await this.redis.get(key);
      if (!data) return null;
      return JSON.parse(data) as T;
    } catch (error) {
      this.logger.warn(`Cache get error for key ${key}:`, error);
      return null;
    }
  }

  /**
   * Set value in cache with TTL
   */
  async set(
    key: string,
    value: unknown,
    ttl: number = this.DEFAULT_TTL
  ): Promise<void> {
    try {
      await this.redis.setex(key, ttl, JSON.stringify(value));
    } catch (error) {
      this.logger.warn(`Cache set error for key ${key}:`, error);
    }
  }

  /**
   * Delete key from cache
   */
  async delete(key: string): Promise<void> {
    try {
      await this.redis.del(key);
    } catch (error) {
      this.logger.warn(`Cache delete error for key ${key}:`, error);
    }
  }

  /**
   * Delete multiple keys matching a pattern
   */
  async deletePattern(pattern: string): Promise<void> {
    try {
      const keys = await this.redis.keys(pattern);
      if (keys.length > 0) {
        await this.redis.del(...keys);
      }
    } catch (error) {
      this.logger.warn(`Cache delete pattern error for ${pattern}:`, error);
    }
  }
}
```

### Pattern 2: Cache-Aside (Lazy Loading)

This is the most common pattern - check cache first, if miss, fetch from DB and cache:

```typescript
@Injectable()
export class UserService {
  constructor(
    private readonly cacheService: CacheService,
    private readonly userRepository: UserRepository
  ) {}

  async getUser(userId: string): Promise<User> {
    // 1. Try cache first
    const cacheKey = `user:${userId}`;
    const cached = await this.cacheService.get<User>(cacheKey);
    if (cached) {
      return cached; // Cache hit!
    }

    // 2. Cache miss - fetch from database
    const user = await this.userRepository.findById(userId);
    if (!user) {
      throw new NotFoundException('User not found');
    }

    // 3. Store in cache for next time
    await this.cacheService.set(cacheKey, user, 3600); // 1 hour TTL

    return user;
  }
}
```

### Pattern 3: Write-Through Cache

Update both cache and database:

```typescript
async updateUser(userId: string, data: UpdateUserDto): Promise<User> {
  // 1. Update database
  const user = await this.userRepository.update(userId, data);

  // 2. Update cache
  const cacheKey = `user:${userId}`;
  await this.cacheService.set(cacheKey, user, 3600);

  return user;
}
```

### Pattern 4: Write-Behind (Write-Back)

Write to cache immediately, write to DB asynchronously:

```typescript
async createUser(data: CreateUserDto): Promise<User> {
  // 1. Create in database
  const user = await this.userRepository.create(data);

  // 2. Cache immediately (don't wait)
  const cacheKey = `user:${user.id}`;
  this.cacheService.set(cacheKey, user, 3600).catch((err) => {
    this.logger.warn('Cache write failed:', err);
  });

  return user;
}
```

### Pattern 5: Invalidation

When data changes, invalidate related cache:

```typescript
async invalidateUser(userId: string): Promise<void> {
  // Delete all user-related cache
  await Promise.all([
    this.cacheService.delete(`user:${userId}`),
    this.cacheService.delete(`user:preferences:${userId}`),
    this.cacheService.deletePattern(`user:${userId}:*`), // All user-related keys
  ]);
}

// Use after updates
async updateUser(userId: string, data: UpdateUserDto): Promise<User> {
  const user = await this.userRepository.update(userId, data);

  // Invalidate old cache
  await this.invalidateUser(userId);

  // Set new cache
  await this.cacheService.set(`user:${userId}`, user, 3600);

  return user;
}
```

---

## Error Handling

### Best Practice: Always Handle Errors Gracefully

```typescript
@Injectable()
export class CacheService {
  private readonly logger = new Logger(CacheService.name);

  constructor(@Inject('REDIS_CLIENT') private readonly redis: Redis) {}

  async get<T>(key: string): Promise<T | null> {
    try {
      const data = await this.redis.get(key);
      if (!data) return null;
      return JSON.parse(data) as T;
    } catch (error) {
      // Log error but don't crash the app
      this.logger.error(`Cache get error for key ${key}:`, error);
      return null; // Return null so app can fallback to DB
    }
  }

  async set(key: string, value: unknown, ttl: number = 600): Promise<boolean> {
    try {
      await this.redis.setex(key, ttl, JSON.stringify(value));
      return true;
    } catch (error) {
      this.logger.error(`Cache set error for key ${key}:`, error);
      return false; // Indicate failure but don't throw
    }
  }
}
```

### Using Cache with Fallback

```typescript
async getUser(userId: string): Promise<User> {
  // Try cache first
  const cached = await this.cacheService.get<User>(`user:${userId}`);
  if (cached) {
    return cached;
  }

  // Fallback to database (even if cache fails, app still works)
  const user = await this.userRepository.findById(userId);

  // Try to cache (but don't fail if it doesn't work)
  await this.cacheService.set(`user:${userId}`, user, 3600).catch(() => {
    // Silently fail - caching is optional
  });

  return user;
}
```

---

## Best Practices

### 1. Key Naming Conventions

Use consistent, hierarchical key names:

```typescript
// Good
'user:123';
'user:123:preferences';
'session:abc123';
'cache:template:456';

// Bad
'user123'; // No separator
'User_123'; // Inconsistent case
'user-123'; // Dash is less common
```

### 2. Always Set TTL

Never store data without expiration (except for truly permanent data):

```typescript
// Good - always set TTL
await this.redis.setex('key', 3600, 'value');

// Bad - no expiration (can fill up memory)
await this.redis.set('key', 'value');
```

### 3. Use Appropriate TTL Values

```typescript
// User data: 1 hour
await this.cacheService.set(`user:${id}`, user, 3600);

// Session: 24 hours
await this.cacheService.set(`session:${token}`, session, 86400);

// Frequently changing data: 5 minutes
await this.cacheService.set(`stats:${id}`, stats, 300);

// Static data: 1 day
await this.cacheService.set(`config:${key}`, config, 86400);
```

### 4. Avoid KEYS in Production

`KEYS` command blocks Redis. Use `SCAN` instead:

```typescript
// Bad - blocks Redis
const keys = await this.redis.keys('user:*');

// Good - non-blocking
let cursor = '0';
const keys: string[] = [];
do {
  const [nextCursor, results] = await this.redis.scan(
    cursor,
    'MATCH',
    'user:*',
    'COUNT',
    100
  );
  cursor = nextCursor;
  keys.push(...results);
} while (cursor !== '0');
```

### 5. Serialize Complex Data

Always stringify objects:

```typescript
// Good
await this.redis.set('user:123', JSON.stringify(user));

// Bad - will store [object Object]
await this.redis.set('user:123', user);
```

### 6. Handle Connection Errors

```typescript
// In redis.module.ts
redisClient.on('error', err => {
  console.error('❌ Redis error:', err.message);
  // Don't throw - let the app continue without cache
});

// In your service
try {
  await this.redis.get('key');
} catch (error) {
  // Fallback to database or return null
  this.logger.warn('Redis unavailable, using fallback');
}
```

### 7. Use Pipelining for Multiple Operations

If you need to do multiple operations, use pipelining:

```typescript
// Bad - multiple round trips
await this.redis.set('key1', 'value1');
await this.redis.set('key2', 'value2');
await this.redis.set('key3', 'value3');

// Good - single round trip
const pipeline = this.redis.pipeline();
pipeline.set('key1', 'value1');
pipeline.set('key2', 'value2');
pipeline.set('key3', 'value3');
await pipeline.exec();
```

### 8. Monitor Memory Usage

```typescript
// Check memory usage
const info = await this.redis.info('memory');
console.log(info);

// Get database size
const dbsize = await this.redis.dbsize();
console.log(`Keys in database: ${dbsize}`);
```

---

## Real-World Examples from This Project

### Example 1: Rate Limiting (Throttler)

See `api-gateway/src/throttler/redis-storage.service.ts`:

```typescript
async increment(key: string, ttl: number): Promise<number> {
  // Increment counter
  const requests = await this.redisClient.incr(key);

  // Set expiration on first request
  if (requests === 1) {
    await this.redisClient.expire(key, ttl);
  }

  return requests;
}
```

### Example 2: User Cache Service

See `services/user-service/src/common/cache.service.ts`:

```typescript
// Get user from cache
const user = await this.cacheService.get<User>(`user:${userId}`);

// Cache user data
await this.cacheService.set(`user:${userId}`, user, 3600);

// Invalidate on update
await this.cacheService.invalidateUser(userId);
```

### Example 3: Pattern-Based Invalidation

```typescript
// Delete all keys matching pattern
async deletePattern(pattern: string): Promise<void> {
  const keys = await this.redis.keys(pattern);
  if (keys.length > 0) {
    await this.redis.del(...keys);
  }
}

// Usage
await this.cacheService.deletePattern('user:123:*');
```

---

## Troubleshooting

### Problem: "Connection refused"

**Solution:**

```bash
# Check if Redis is running
redis-cli ping

# Start Redis
brew services start redis
# Or
docker-compose up redis -d
```

### Problem: "Invalid password"

**Solution:**

```bash
# Check your REDIS_URL in .env
# Format: redis://:password@host:port
REDIS_URL=redis://:your_password@localhost:6379
```

### Problem: "Memory limit exceeded"

**Solution:**

- Set TTL on all keys
- Use `redis-cli --bigkeys` to find large keys
- Increase Redis memory limit in config
- Implement key eviction policy

### Problem: "KEYS command is slow"

**Solution:**

- Use `SCAN` instead of `KEYS`
- Use specific key patterns instead of wildcards
- Consider using Redis Sets to track keys

### Problem: "Data not persisting"

**Solution:**

- Check if persistence is enabled: `redis-cli CONFIG GET save`
- Enable persistence in redis.conf or docker-compose

---

## Quick Reference

### Common Commands

```typescript
// Strings
await redis.set('key', 'value', 'EX', 3600);
await redis.get('key');
await redis.del('key');

// Hashes
await redis.hset('hash', 'field', 'value');
await redis.hget('hash', 'field');
await redis.hgetall('hash');

// Lists
await redis.rpush('list', 'item');
await redis.lrange('list', 0, -1);

// Sets
await redis.sadd('set', 'member');
await redis.smembers('set');

// Counters
await redis.incr('counter');
await redis.incrby('counter', 5);

// TTL
await redis.expire('key', 3600);
await redis.ttl('key');
```

### Common Patterns

```typescript
// Cache with TTL
await redis.setex('key', 3600, JSON.stringify(data));

// Get or set
const value = (await redis.get('key')) || (await fetchAndSet());

// Atomic increment
const count = await redis.incr('counter');
if (count === 1) await redis.expire('counter', 60);

// Check and set
const exists = await redis.exists('key');
if (!exists) await redis.set('key', 'value');
```

---

## Summary

1. **Setup**: Install Redis locally or use Docker
2. **Module**: Create `RedisModule` with `@Global()` decorator
3. **Inject**: Use `@Inject('REDIS_CLIENT')` in services
4. **Operations**: Use `get`, `set`, `del`, `setex` for basic operations
5. **TTL**: Always set expiration times
6. **Error Handling**: Wrap operations in try-catch
7. **Patterns**: Use cache-aside, write-through, or write-behind
8. **Best Practices**: Consistent key naming, appropriate TTLs, avoid KEYS command

Remember: **Redis is a cache, not a database**. Always have a fallback to your primary database!
