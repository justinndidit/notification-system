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
        return new Redis(redisUrl || 'redis://localhost:6379');
      },
    },
  ],
  exports: ['REDIS_CLIENT'],
})
export class RedisModule {}
