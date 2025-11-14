import { Module, Global } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import Redis from 'ioredis';
import config from '../config/config';

const { redisUrl } = config();

@Global()
@Module({
  imports: [ConfigModule],
  providers: [
    {
      provide: 'REDIS_CLIENT',
      useFactory: () => {
        const connectionString = redisUrl || 'redis://localhost:6379';

        // ioredis automatically handles connection strings with credentials
        // For Redis Cloud, the URL format is: redis://username:password@host:port
        const redisClient = new Redis(connectionString, {
          // Enable retry strategy for better reliability
          retryStrategy: (times) => {
            const delay = Math.min(times * 50, 2000);
            return delay;
          },
          // Enable reconnection
          enableReadyCheck: true,
          maxRetriesPerRequest: 3,
        });

        // Log connection events for debugging
        redisClient.on('connect', () => {
          console.log('✅ [User Service] Redis client connected');
        });

        redisClient.on('ready', () => {
          console.log('✅ [User Service] Redis client ready');
        });

        redisClient.on('error', (err) => {
          console.error('❌ [User Service] Redis client error:', err.message);
        });

        redisClient.on('close', () => {
          console.log('⚠️  [User Service] Redis client connection closed');
        });

        redisClient.on('reconnecting', () => {
          console.log('🔄 [User Service] Redis client reconnecting...');
        });

        return redisClient;
      },
    },
  ],
  exports: ['REDIS_CLIENT'],
})
export class RedisModule {}
