// src/throttler/redis.module.ts
import { Module } from '@nestjs/common';
import Redis from 'ioredis';
import config from '../config/config';

const { redisUrl } = config();

@Module({
  providers: [
    {
      provide: 'REDIS_CLIENT',
      useFactory: () => new Redis(redisUrl || 'redis://localhost:6379'),
    },
  ],
  exports: ['REDIS_CLIENT'],
})
export class RedisModule {}
