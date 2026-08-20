import { Module } from '@nestjs/common';
import { TemplateController } from './template.controller';
import { PrismaModule } from 'src/prisma/prisma.module';
import { TemplateService } from './template.service';
import { AuthModule } from '../auth/auth.module';
import { CacheService } from '../common/cache.service';
import { ServiceOrJwtGuard } from '../common/service-or-jwt.guard';

@Module({
  imports: [PrismaModule, AuthModule],
  controllers: [TemplateController],
  providers: [TemplateService, CacheService, ServiceOrJwtGuard],
  exports: [TemplateService],
})
export class TemplateModule {}
