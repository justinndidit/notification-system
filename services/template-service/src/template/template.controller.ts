import {
  Controller,
  Post,
  Body,
  Req,
  UnauthorizedException,
} from '@nestjs/common';
import { UseGuards } from '@nestjs/common';
import { TemplateService } from './template.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';
import { CreateTemplateDto } from './dto/create.template.dto';
import type { JwtRequest } from 'src/types/types';

@UseGuards(JwtAuthGuard)
@Controller('template')
export class TemplateController {
  constructor(private readonly templatesService: TemplateService) {}
  @Post()
  create(@Body() createTemplateDto: CreateTemplateDto, @Req() req: JwtRequest) {
    const role = req.user.role;
    if (role !== 'admin') {
      throw new UnauthorizedException(
        'Forbidden: You are not authorized to create a template',
      );
    }
    return this.templatesService.create(createTemplateDto);
  }
}
