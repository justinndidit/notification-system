import { Injectable, Inject, Logger } from '@nestjs/common';
import Redis from 'ioredis';

@Injectable()
export class CacheService {
  private readonly logger = new Logger(CacheService.name);
  private readonly DEFAULT_TTL = 1800; // 30 minutes in seconds

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
    ttl: number = this.DEFAULT_TTL,
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

  /**
   * Invalidate template-related cache
   */
  async invalidateTemplate(
    templateId: string,
    event?: string,
    channel?: string,
    language?: string,
  ): Promise<void> {
    const promises = [
      // Invalidate template by ID (both full and latest versions)
      this.deletePattern(`template:${templateId}:*`),
      this.delete(`template:${templateId}`),
    ];

    if (event && channel && language) {
      promises.push(
        this.delete(`template:event:${event}:${channel}:${language}`),
      );
    }

    // Also invalidate all event-based caches for this template
    promises.push(
      this.deletePattern(
        `template:event:*:${channel || '*'}:${language || '*'}`,
      ),
    );

    await Promise.all(promises);
  }
}
