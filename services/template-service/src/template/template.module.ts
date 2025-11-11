import { Module } from '@nestjs/common';
import { TemplateController } from './template.controller';
import { PrismaModule } from 'src/prisma/prisma.module';
import { TemplateService } from './template.service';
import { AuthModule } from '../auth/auth.module';

@Module({
  imports: [PrismaModule, AuthModule],
  controllers: [TemplateController],
  providers: [TemplateService],
  exports: [TemplateService],
})
export class TemplateModule {}
