import { Module } from '@nestjs/common';
import { PrismaModule } from 'src/prisma/prisma.module';
import { PassportModule } from '@nestjs/passport';
import { JwtModule } from '@nestjs/jwt';
import { ConfigModule, ConfigService } from '@nestjs/config';
import { JwtStrategy } from './jwt.strategy';
import { UserService } from './user.service';
import { UserController } from './user.controller';
import { CacheService } from '../common/cache.service';

@Module({
  imports: [
    PrismaModule,
    PassportModule,
    JwtModule.registerAsync({
      imports: [ConfigModule],
      useFactory: (configService: ConfigService) => ({
        secret: configService.get<string>('JWT_SECRET'), // JWT secret from config
        signOptions: { expiresIn: '1h' }, // Token expiration time
      }),
      inject: [ConfigService],
    }),
    // Import ConfigModule to access environment variables and configuration
    ConfigModule,
  ],

  providers: [UserService, JwtStrategy, CacheService],
  controllers: [UserController],
  exports: [JwtStrategy, PassportModule],
})
export class UserModule {}
