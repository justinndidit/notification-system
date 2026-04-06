import { Module, Global } from '@nestjs/common';
import { ConfigModule, ConfigService } from '@nestjs/config';
import Redis from 'ioredis';
@Global()
@Module({
  imports: [ConfigModule],
  providers: [
    {
      provide: 'REDIS_CLIENT',
      inject: [ConfigService],
      useFactory: (configService : ConfigService) => {
        const connectionString = configService.get<string>('redisUrl') || "";

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
          console.log('✅ [Template Service] Redis client connected');
        });

        redisClient.on('ready', () => {
          console.log('✅ [Template Service] Redis client ready');
        });

        redisClient.on('error', (err) => {
          console.log(connectionString)
          console.error(
            '❌ [Template Service] Redis client error:',
            err.message,
          );
        });

        redisClient.on('close', () => {
          console.log('⚠️  [Template Service] Redis client connection closed');
        });

        redisClient.on('reconnecting', () => {
          console.log('🔄 [Template Service] Redis client reconnecting...');
        });

        return redisClient;
      },
    },
  ],
  exports: ['REDIS_CLIENT'],
})
export class RedisModule {}
