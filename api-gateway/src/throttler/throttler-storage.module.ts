// src/throttler/throttler-storage.module.ts
import { Module } from '@nestjs/common';
import { CustomRedisStorageService } from './redis-storage.service';
import { RedisModule } from './redis.module';

@Module({
  imports: [RedisModule],
  providers: [CustomRedisStorageService],
  exports: [CustomRedisStorageService],
})
export class ThrottlerStorageModule {}
