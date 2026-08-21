import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { ConfigModule } from '@nestjs/config';
import { ThrottlerModule, ThrottlerGuard } from '@nestjs/throttler';
import { CustomRedisStorageService } from './throttler/redis-storage.service';
import { APP_GUARD, APP_INTERCEPTOR } from '@nestjs/core';
import { JwtAuthGuard } from './auth/jwt-auth.guard';
import { LoggingInterceptor } from './middleware/logging.interceptor';
import { ThrottlerStorageModule } from './throttler/throttler-storage.module';
import { ProxyModule } from './proxy/proxy.module';
// import { NotificationModule } from './notification/notification.module';
import { AuthModule } from './auth/auth.module';
import { ResponseInterceptor } from './common/interceptors/response.interceptors';
import { RedisModule } from './common/redis.module';
import { Reflector } from '@nestjs/core';
import config from './config/config';

const appConfig = config();

@Module({
  imports: [
    RedisModule,
    ProxyModule,
    AuthModule,
    // NotificationModule,
    ConfigModule.forRoot({ isGlobal: true }),
    ThrottlerModule.forRootAsync({
      imports: [ThrottlerStorageModule],
      inject: [CustomRedisStorageService],
      useFactory: (storage: CustomRedisStorageService) => ({
        throttlers: [
          {
            ttl: appConfig.throttleTtl * 1000,
            limit: appConfig.throttleLimit,
          },
        ],
        storage,
      }),
    }),
  ],
  controllers: [AppController],
  providers: [
    AppService,
    Reflector,
    // Order matters: rate limiting runs before authentication so unauthenticated
    // floods are shed at the edge rather than after JWT verification.
    { provide: APP_GUARD, useClass: ThrottlerGuard },
    { provide: APP_GUARD, useClass: JwtAuthGuard },
    { provide: APP_INTERCEPTOR, useClass: ResponseInterceptor },
    CustomRedisStorageService,
    LoggingInterceptor,
  ],
  exports: [CustomRedisStorageService],
})
export class AppModule {}
