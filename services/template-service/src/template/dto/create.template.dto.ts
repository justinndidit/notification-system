// src/templates/dto/create-template.dto.ts
import { PartialType } from '@nestjs/mapped-types';
import { NotificationChannel, TemplateEvent } from '@prisma/client';
import {
  IsEnum,
  IsString,
  IsOptional,
  IsBoolean,
  IsJSON,
} from 'class-validator';

export class CreateTemplateDto {
  @IsString()
  name: string;

  @IsEnum(TemplateEvent)
  event: TemplateEvent;

  @IsEnum(NotificationChannel)
  channel: NotificationChannel;

  @IsString()
  language: string;

  @IsOptional()
  @IsString()
  subject?: string; // EMAIL only

  @IsOptional()
  @IsString()
  title?: string; // PUSH only

  @IsString()
  body: string; // Handlebars template

  @IsOptional()
  @IsJSON()
  variables?: Record<string, string>; // e.g., { "user.name": "string" }
}

export class UpdateTemplateDto extends PartialType(CreateTemplateDto) {
  @IsOptional()
  @IsBoolean()
  isActive?: boolean;
}

export class RenderTemplateDto {
  @IsString()
  templateId: string; // Or event/channel/lang combo

  @IsJSON()
  data: Record<string, any>; // Vars to substitute, e.g., { user: { name: 'John' }, order: { id: '123' } }
}
