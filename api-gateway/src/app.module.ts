import { Module } from '@nestjs/common';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { ConfigModule } from '@nestjs/config';
import { ThrottlerModule } from '@nestjs/throttler';
import { CustomRedisStorageService } from './throttler/redis-storage.service';
import { APP_GUARD, APP_INTERCEPTOR } from '@nestjs/core';
import { JwtAuthGuard } from './auth/jwt-auth.guard';
import { LoggingInterceptor } from './middleware/logging.interceptor';
import { ThrottlerStorageModule } from './throttler/throttler-storage.module';
import { ProxyModule } from './middleware/proxy.module';
import { NotificationModule } from './notification/notification.module';
import { AuthModule } from './auth/auth.module';
import { ResponseInterceptor } from './common/interceptors/response.interceptors';
import { RedisModule } from './common/redis.module';
import { Reflector } from '@nestjs/core';
import { JwtHelper } from './common/jwt-helper';

@Module({
  imports: [
    RedisModule,
    ProxyModule,
    AuthModule,
    NotificationModule,
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
    Reflector,
    JwtHelper,
    { provide: APP_GUARD, useClass: JwtAuthGuard },
    { provide: APP_INTERCEPTOR, useClass: ResponseInterceptor },
    CustomRedisStorageService,
    LoggingInterceptor,
  ],
  exports: [CustomRedisStorageService],
})
export class AppModule {}
