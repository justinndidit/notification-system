import { Module } from '@nestjs/common';
import { TemplateController } from './template.controller';
import { PrismaModule } from 'src/prisma/prisma.module';
import { TemplateService } from './template.service';
import { AuthModule } from '../auth/auth.module';
import { CacheService } from '../common/cache.service';

@Module({
  imports: [PrismaModule, AuthModule],
  controllers: [TemplateController],
  providers: [TemplateService, CacheService],
  exports: [TemplateService],
})
export class TemplateModule {}
