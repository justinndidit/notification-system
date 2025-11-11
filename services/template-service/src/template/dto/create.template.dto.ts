// src/templates/dto/create-template.dto.ts
import { PartialType } from '@nestjs/mapped-types';
import { NotificationChannel } from '@prisma/client';
import {
  IsEnum,
  IsString,
  IsOptional,
  IsBoolean,
  IsObject,
} from 'class-validator';

export class CreateTemplateDto {
  @IsString()
  name: string;

  @IsString()
  event: string;

  @IsEnum(NotificationChannel, { each: true })
  channel: NotificationChannel[];

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

  @IsObject()
  @IsOptional()
  variables?: Record<string, string>; // e.g., { "user.name": "string" }
}

export class UpdateTemplateDto extends PartialType(CreateTemplateDto) {
  @IsOptional()
  @IsBoolean()
  isActive?: boolean;
}

export class RenderTemplateDto {
  @IsObject()
  data: Record<string, any>; // Vars to substitute, e.g., { user: { name: 'John' }, order: { id: '123' } }
}
