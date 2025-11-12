import { Injectable, Inject } from '@nestjs/common';
import { ThrottlerStorage } from '@nestjs/throttler';
import Redis from 'ioredis';

// Define full record structure expected by NestJS v11
interface ThrottlerStorageRecord {
  totalHits: number;
  timeToExpire: number;
  blockDuration: number;
  key: string;
  limit: number;
  isBlocked: boolean;
  timeToBlockExpire: number;
}

@Injectable()
export class CustomRedisStorageService implements ThrottlerStorage {
  constructor(@Inject('REDIS_CLIENT') private readonly redisClient: Redis) {}

  async increment(
    key: string,
    ttl: number,
    limit: number,
    blockDuration: number,
    throttlerName: string,
  ): Promise<ThrottlerStorageRecord> {
    const recordKey = `${throttlerName}:${key}`;
    const blockKey = `${recordKey}:block`;

    // If this key is currently blocked, return blocked status
    const blockedTtl = await this.redisClient.ttl(blockKey);
    if (blockedTtl > 0) {
      return {
        totalHits: limit,
        timeToExpire: 0,
        blockDuration,
        key,
        limit,
        isBlocked: true,
        timeToBlockExpire: blockedTtl,
      };
    }

    // Increment the request count
    const requests = await this.redisClient.incr(recordKey);
    if (requests === 1) {
      await this.redisClient.expire(recordKey, ttl);
    }

    const timeToExpire = await this.redisClient.ttl(recordKey);

    // If limit exceeded, set block duration
    if (requests > limit) {
      await this.redisClient.set(blockKey, '1', 'EX', blockDuration);
      return {
        totalHits: requests,
        timeToExpire,
        blockDuration,
        key,
        limit,
        isBlocked: true,
        timeToBlockExpire: blockDuration,
      };
    }

    // Otherwise, return normal record
    return {
      totalHits: requests,
      timeToExpire,
      blockDuration,
      key,
      limit,
      isBlocked: false,
      timeToBlockExpire: 0,
    };
  }
}
