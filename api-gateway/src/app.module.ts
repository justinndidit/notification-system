import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { ConfigModule } from '@nestjs/config';
import { ThrottlerModule } from '@nestjs/throttler';
import { CustomRedisStorageService } from './throttler/redis-storage.service';
import Redis from 'ioredis';
import { APP_GUARD } from '@nestjs/core';
import { JwtAuthGuard } from './auth/jwt-auth.guard';
import config from './config/config';
import { LoggingInterceptor } from './middleware/logging.interceptor';
import { ThrottlerStorageModule } from './throttler/throttler-storage.module';
import { ProxyModule } from './middleware/proxy.module';

const { redisUrl } = config();

@Module({
  imports: [
    ProxyModule,
    ConfigModule.forRoot({ isGlobal: true }),
    ThrottlerModule.forRootAsync({
      imports: [ThrottlerStorageModule],
      inject: [CustomRedisStorageService],
      useFactory: (storage: CustomRedisStorageService) => ({
        throttlers: [
          {
            ttl: 60,
            limit: 100,
          },
        ],
        storage,
      }),
    }),
  ],
  controllers: [AppController],
  providers: [
    AppService,
    {
      provide: 'REDIS_CLIENT',
      useFactory: () => new Redis(redisUrl || 'redis://localhost:6379'),
    },
    { provide: APP_GUARD, useClass: JwtAuthGuard },
    CustomRedisStorageService,
    LoggingInterceptor,
  ],
  exports: [CustomRedisStorageService],
})
export class AppModule {}
